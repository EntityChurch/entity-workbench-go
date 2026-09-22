#!/usr/bin/env python3
"""crash-stack-report — read an ELF coredump and answer, with numbers,
the three questions a .NET GUI crash always turns on.

    1. WHICH THREAD died, and was it the UI thread?
    2. HOW DEEP was it, and how much stack did it actually use?
       (i.e. was this a stack overflow, or a normal-depth walk that
       faulted for some other reason?)
    3. WHAT are the anonymous frames? (JIT code, resolved through the
       runtime's perf map, if one was written.)

WHY THIS EXISTS
---------------
On 2026-09-01 a SIGSEGV was classified as a stack overflow because
`coredumpctl info` printed 45 consecutive frames returning to a single
address and systemd truncated the trace, so "the real depth is larger"
looked like a safe inference. It was not. The frames are a uniform 448
bytes and there are exactly 45 of them — 19.2 KB, inside a thread that
had used 26 KB in total. A 45-deep walk over a UI tree is an ordinary
depth, not a runaway one, and the number was sitting in the core the
whole time.

The general lesson, which is the reason this is a script and not a
paragraph in a doctrine: **a frame COUNT from a truncating pretty-printer
is a lower bound; the stack itself is the measurement.** The same shape
as AP15 (a fail-fast test runner's failure count is a lower bound), one
layer down. Reading it by hand takes an hour and is easy to get wrong, so
it is automated and printed in full.

WHAT A CLEAN RESULT DOES NOT MEAN
---------------------------------
This reports stack geometry. It does not name the fault: on .NET/Linux
the signal that reaches a systemd core has usually been RE-RAISED by
CoreCLR's handler (si_code 128 / SI_KERNEL, si_addr 0), so the register
context is not the fault site (AP34). Believe the stack layout here;
do not believe the reported rip is where it crashed. For an actual
faulting context, catch the first SIGSEGV live under gdb, or read the
managed minidump createdump writes (both in DOCTRINE-CRASH-FORENSICS).

Usage:
    crash-stack-report.py <corefile> [--perf-map /tmp/perf-<pid>.map]
"""

import os
import re
import struct
import sys
from collections import Counter

# ---- ELF core parsing ---------------------------------------------------

PT_LOAD, PT_NOTE = 1, 4
NT_PRSTATUS, NT_FILE = 1, 0x46494c45

# user_regs_struct (x86-64) index of each register we care about.
# fs_base is the thread descriptor (TCB) address, which glibc places at
# the TOP of the thread's stack — the one anchor that stays valid when
# the recorded rsp does not. See the fallback in analyse_thread.
REG_RBP, REG_RIP, REG_RSP, REG_FSBASE = 4, 16, 19, 21
PRSTATUS_REG_OFFSET = 112          # bytes into elf_prstatus
PRSTATUS_CURSIG_OFFSET = 12        # pr_cursig, 2 bytes
PRSTATUS_PID_OFFSET = 32           # pr_pid, 4 bytes


class Core:
    def __init__(self, path):
        self.f = open(path, "rb")
        self.path = path
        data = self.f.read(64)
        if data[:4] != b"\x7fELF" or data[4] != 2:
            raise SystemExit(f"{path}: not a 64-bit ELF file")
        e_phoff, = struct.unpack_from("<Q", data, 32)
        e_phentsize, e_phnum = struct.unpack_from("<HH", data, 54)
        self.segments = []   # (vaddr, vaddr+filesz, file_offset)
        self.notes = []
        self.f.seek(e_phoff)
        phdrs = self.f.read(e_phentsize * e_phnum)
        for i in range(e_phnum):
            p = i * e_phentsize
            p_type, p_flags = struct.unpack_from("<II", phdrs, p)
            p_offset, p_vaddr = struct.unpack_from("<QQ", phdrs, p + 8)
            p_filesz, p_memsz = struct.unpack_from("<QQ", phdrs, p + 32)
            if p_type == PT_LOAD and p_filesz:
                self.segments.append((p_vaddr, p_vaddr + p_filesz, p_offset, p_flags))
            elif p_type == PT_NOTE:
                self.notes.append((p_offset, p_filesz))
        self.segments.sort()

    def read(self, vaddr, n):
        for lo, hi, off, _flags in self.segments:
            if lo <= vaddr and vaddr + n <= hi:
                self.f.seek(off + (vaddr - lo))
                return self.f.read(n)
        return None

    def mapped(self, vaddr):
        return any(lo <= vaddr < hi for lo, hi, _o, _f in self.segments)

    def iter_notes(self):
        for off, size in self.notes:
            self.f.seek(off)
            blob = self.f.read(size)
            p = 0
            while p + 12 <= len(blob):
                namesz, descsz, ntype = struct.unpack_from("<III", blob, p)
                p += 12
                name = blob[p:p + namesz]
                p += (namesz + 3) & ~3
                desc = blob[p:p + descsz]
                p += (descsz + 3) & ~3
                yield ntype, name, desc

    def threads(self):
        """Yield (tid, cursig, regs dict) per NT_PRSTATUS, in core order.
        The FIRST prstatus is the thread the kernel dumped for, i.e. the
        one that took the signal."""
        out = []
        for ntype, _name, desc in self.iter_notes():
            if ntype != NT_PRSTATUS or len(desc) < PRSTATUS_REG_OFFSET + 27 * 8:
                continue
            cursig, = struct.unpack_from("<H", desc, PRSTATUS_CURSIG_OFFSET)
            tid, = struct.unpack_from("<I", desc, PRSTATUS_PID_OFFSET)
            regs = struct.unpack_from("<27Q", desc, PRSTATUS_REG_OFFSET)
            out.append((tid, cursig, {
                "rsp": regs[REG_RSP], "rip": regs[REG_RIP], "rbp": regs[REG_RBP],
                "fs_base": regs[REG_FSBASE]}))
        return out

    def file_mappings(self):
        """NT_FILE: [(start, end, path)] for every file-backed mapping."""
        for ntype, _name, desc in self.iter_notes():
            if ntype != NT_FILE:
                continue
            count, _pgsz = struct.unpack_from("<QQ", desc, 0)
            entries = []
            p = 16
            for _ in range(count):
                start, end, _off = struct.unpack_from("<QQQ", desc, p)
                p += 24
                entries.append([start, end, None])
            names = desc[p:].split(b"\x00")
            for i, ent in enumerate(entries):
                if i < len(names):
                    ent[2] = names[i].decode("utf-8", "replace")
            return [(a, b, c) for a, b, c in entries]
        return []


# ---- perf map -----------------------------------------------------------

def load_perf_map(path):
    """The runtime writes `<start-hex> <size-hex> <name>` per JIT'd method
    when DOTNET_PerfMapEnabled=1. Without one, every managed frame in a
    coredump is an anonymous address."""
    if not path or not os.path.exists(path):
        return []
    out = []
    with open(path, "r", errors="replace") as fh:
        for line in fh:
            parts = line.split(None, 2)
            if len(parts) < 3:
                continue
            try:
                start = int(parts[0], 16)
                size = int(parts[1], 16)
            except ValueError:
                continue
            out.append((start, start + size, parts[2].rstrip()))
    out.sort()
    return out


def guess_perf_map(core_path):
    m = re.search(r"\.(\d+)\.\d+", os.path.basename(core_path))
    if m:
        cand = f"/tmp/perf-{m.group(1)}.map"
        if os.path.exists(cand):
            return cand
    return None


# ---- symbolization ------------------------------------------------------

    # A stack is full of data that looks like a pointer: object
    # references, saved arguments, boxed doubles. Only values inside a
    # mapping that can hold CODE are return addresses, and mistaking the
    # two puts a repeated object reference at the top of the recursion
    # report — which it did on the first run of this script.
    #
    # Native code is in a .so or the main executable; JIT code is in
    # CoreCLR's W^X double mapping (an unlinked memfd, so NT_FILE names
    # it "memfd:doublemapper (deleted)"); ReadyToRun code is inside the
    # managed .dll images.
CODE_NAME = re.compile(r"(\.so(\.|$)|\.dll$|doublemapper|entity-avalonia$)")


class Symbolizer:
    def __init__(self, files, perf):
        self.files = files
        self.perf = perf
        self.code = [(lo, hi, p) for lo, hi, p in files if CODE_NAME.search(p or "")]

    def name(self, addr):
        for lo, hi, sym in self.perf:
            if lo <= addr < hi:
                return f"{sym} +0x{addr - lo:x}"
        for lo, hi, path in self.files:
            if lo <= addr < hi:
                return f"{os.path.basename(path)} +0x{addr - lo:x}"
        return "<anonymous — not a file mapping>"

    def is_code(self, addr):
        return any(lo <= addr < hi for lo, hi, _p in self.code)


# ---- the actual analysis ------------------------------------------------

PAGE = 0x1000


def stack_top(core, rsp, limit_pages=8192):
    """Walk up from rsp to the first unmapped page. For a pthread stack
    the thread descriptor sits at the top, so this is the stack base."""
    page = rsp & ~(PAGE - 1)
    top = page
    for _ in range(limit_pages):
        if not core.mapped(top + PAGE):
            break
        top += PAGE
    return top + PAGE


# How far below the thread descriptor to scan when we have to fall back
# to fs_base. Comfortably more than any sane frame chain, far less than a
# whole 8 MB stack.
FALLBACK_SCAN_BYTES = 2 << 20


def analyse_thread(core, sym, tid, cursig, regs, verbose):
    rsp = regs["rsp"]
    top = regs["fs_base"]
    fell_back = False

    if not core.mapped(rsp):
        # THE COMMON CASE FOR A .NET CRASH, and worth calling out rather
        # than skipping the thread. CoreCLR's signal handler runs on the
        # thread's ALTERNATE signal stack, and systemd-coredump does not
        # dump it — so the register set the core records points at memory
        # the core does not contain. Recorded rsp: useless. Recorded rip:
        # useless (AP34). The thread's real stack is still there, and
        # fs_base still points at the top of it.
        print(f"  !! recorded rsp 0x{rsp:x} is NOT PRESENT in this core.")
        print("     That is the signature of a fault taken through CoreCLR's handler:")
        print("     the handler runs on the alternate signal stack, which systemd does")
        print("     not dump. The recorded registers describe the handler, not the")
        print("     fault. Falling back to fs_base (the thread descriptor) to find the")
        print("     thread's real stack.")
        print()
        print("     READ THE NEXT SENTENCE BEFORE QUOTING ANYTHING BELOW. In this mode")
        print("     there is no live stack pointer, so the scan cannot tell a LIVE frame")
        print("     from stale bytes left by an earlier call on the same thread. A")
        print("     uniform-stride run IS trustworthy (stale frames do not line up on a")
        print("     constant stride by accident). An isolated address is not — in")
        print("     particular, symbols appearing BELOW the deepest uniform run are")
        print("     probably garbage from a previous pass, not callers.")
        fell_back = True
        if not core.mapped(top - PAGE):
            print(f"     fs_base 0x{top:x} is not mapped either — cannot analyse.")
            return
        lo = top - FALLBACK_SCAN_BYTES
    else:
        top = stack_top(core, rsp)
        lo = rsp

    print(f"  fs_base    0x{regs['fs_base']:x}  (thread descriptor = top of this thread's stack)")
    if not fell_back:
        print(f"  rsp        0x{rsp:x}")
        print(f"  stack USED {top - rsp:,} bytes ({(top - rsp) / 1024:.1f} KB)")
    print(f"  rip        0x{regs['rip']:x}  {sym.name(regs['rip'])}")
    print("             NOTE: on .NET this rip is usually the re-raise site, not the")
    print("             fault site (AP34). Trust the geometry below, not this address.")

    # Collect every stack word that could be a return address.
    chunks, a = [], lo & ~(PAGE - 1)
    while a < top:
        c = core.read(a, PAGE)
        chunks.append(c if c is not None else b"\x00" * PAGE)
        a += PAGE
    words = b"".join(chunks)
    base = lo & ~(PAGE - 1)
    addrs = []
    for i in range(0, len(words) - 8, 8):
        v, = struct.unpack_from("<Q", words, i)
        if 0x1000 < v < (1 << 48) and sym.is_code(v):
            addrs.append((base + i, v))

    counts = Counter(v for _a, v in addrs)
    candidates = []
    for v, n in counts.items():
        if n < 4:
            continue
        sites = sorted(a for a, w in addrs if w == v)
        deltas = [sites[i + 1] - sites[i] for i in range(len(sites) - 1)]
        hist = Counter(deltas).most_common(3)
        stride = hist[0][0] if hist else 0
        uniform = bool(hist) and hist[0][1] == len(deltas)
        candidates.append((v, n, sites, hist, stride, uniform))

    if not candidates:
        print("  no repeated return address on this stack — not a recursion.")
        return

    # Rank uniform-stride runs first: a single call site recursing puts
    # its return address at exactly one offset in every frame, so a
    # perfectly uniform stride IS the recursion signature. A mixed stride
    # is a call site reached repeatedly through different paths, which is
    # a different (and usually less interesting) thing.
    candidates.sort(key=lambda c: (not c[5], -c[1]))

    print("\n  RECURSION ANALYSIS (repeated CODE addresses, >=4 occurrences)")
    for v, n, sites, hist, stride, uniform in candidates[:6]:
        span = sites[-1] - sites[0]
        print(f"    0x{v:x}  x{n}   {sym.name(v)}")
        print(f"      frame stride {stride} bytes"
              f"{' (UNIFORM — a single recursing call site)' if uniform else f' (mixed: {hist})'}")
        print(f"      span {span:,} bytes ({span / 1024:.1f} KB), "
              f"deepest at 0x{sites[0]:x}, shallowest at 0x{sites[-1]:x}")
    repeats = [(c[0], c[1]) for c in candidates]

    # The verdict this whole script exists to print.
    deepest = repeats[0]
    sites = sorted(a for a, w in addrs if w == deepest[0])
    total_recursion = sites[-1] - sites[0] if len(sites) > 1 else 0
    # Depth below the thread descriptor is the measurement that survives
    # a useless rsp: it is how far down the stack the frame chain
    # actually reached.
    depth_below_tcb = regs["fs_base"] - sites[0]

    print("\n  VERDICT")
    print(f"    deepest repeating call site occurs {deepest[1]} times, "
          f"consuming {total_recursion:,} bytes")
    print(f"    the frame chain reaches {depth_below_tcb:,} bytes "
          f"({depth_below_tcb / 1024:.1f} KB) below the thread descriptor")
    if depth_below_tcb < 512 * 1024:
        print("    -> NOT A STACK OVERFLOW. Every thread CoreCLR creates gets at least")
        print("       a megabyte of stack, and this chain did not reach a tenth of that.")
        print("       Whatever `coredumpctl info` shows, a repeating return address at")
        print("       this depth is an ORDINARY recursive walk — a UI tree is tens of")
        print("       levels deep by construction. Look for a memory fault inside the")
        print("       walk (a torn read, a freed object, a cross-thread mutation), not")
        print("       for an unbounded recursion.")
    else:
        print("    -> Stack exhaustion is a live hypothesis at this depth. Confirm it by")
        print("       locating the guard page: the fault address should sit in an")
        print("       unmapped page immediately below the deepest frame. Do not settle")
        print("       for the frame count alone.")

    if verbose:
        print("\n  frames (deepest first, deduplicated runs):")
        last, run = None, 0
        for a, v in addrs:
            if v == last:
                run += 1
                continue
            if last is not None and run:
                print(f"    ... x{run}")
            print(f"    0x{a:x} -> 0x{v:x}  {sym.name(v)}")
            last, run = v, 0


def main():
    args = [a for a in sys.argv[1:]]
    if not args or args[0] in ("-h", "--help"):
        print(__doc__)
        return 0
    core_path = args[0]
    perf_path = None
    verbose = "-v" in args or "--verbose" in args
    if "--perf-map" in args:
        perf_path = args[args.index("--perf-map") + 1]
    if perf_path is None:
        perf_path = guess_perf_map(core_path)

    core = Core(core_path)
    perf = load_perf_map(perf_path)
    sym = Symbolizer(core.file_mappings(), perf)

    print(f"core      {core_path}")
    print(f"perf map  {perf_path or '<none>'}"
          f"{f' ({len(perf)} JIT methods)' if perf else ''}")
    if not perf:
        print("          !! No perf map. Managed frames cannot be named.")
        print("             Set DOTNET_PerfMapEnabled=1 before launch (run-with-dump.sh")
        print("             does). Without it this report gives geometry only.")
    threads = core.threads()
    print(f"threads   {len(threads)}")
    print()

    for i, (tid, cursig, regs) in enumerate(threads):
        marker = "  <-- FAULTING (first prstatus in the core)" if i == 0 else ""
        if i > 0 and not verbose:
            continue
        print(f"THREAD {tid}  cursig={cursig}{marker}")
        analyse_thread(core, sym, tid, cursig, regs, verbose)
        print()
    if len(threads) > 1 and not verbose:
        print(f"({len(threads) - 1} other threads not shown; pass -v for all)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
