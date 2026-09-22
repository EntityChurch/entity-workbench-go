// altstack-preload.c — an LD_PRELOAD interposer on sigaltstack(2) that
// gives EVERY thread in the process an alternate signal stack big enough
// for CoreCLR's signal handler chain on this hardware.
//
// ===================================================================
// WHY THIS EXISTS — measured 2026-09-06, on two independent coredumps
// ===================================================================
//
// `make twopeer-gui` crashed peer-a twice in four runs. Both cores are
// byte-for-byte the same shape, and the shape is an alternate-signal-
// stack overflow on a thread that is NEITHER the UI thread NOR the
// render thread:
//
//   core 1053759            core 726963
//   ---------------------   ---------------------
//   rip  libcoreclr+0x3af3da (identical)
//   rbp - rsp = 0x1b30      rbp - rsp = 0x1b30      (identical frame)
//   rsp 0x320 bytes BELOW the usable base  (identical overshoot)
//   used 13,088 bytes of a 12,288-byte usable region (identical)
//   si_code 128 (SI_KERNEL), si_addr 0      (both)
//
// The PAL mmaps 16 KiB per thread, mprotects the low 4 KiB PROT_NONE as
// a guard, and hands the kernel ss_size = 16384 anyway — so the usable
// region is 12 KiB and the guard catches the overrun. In the core the
// crashing thread's rsp sits inside that guard page, and the two DLL
// mappings on either side make the region unmistakable:
//
//   0x7eff1081b000-0x7eff1081c000   4 KiB  ---   GUARD   <== rsp here
//   0x7eff1081c000-0x7eff1081f000  12 KiB  rw-   the alt stack
//
// The 13,088 bytes break down as ~6.1 KiB of kernel signal frame plus
// handler prologue, then ONE ~6.9 KiB frame (rbp-rsp = 0x1b30) in
// CoreCLR's common signal handler, which builds a full CONTEXT on the
// stack. It is not a recursion — there are exactly three return
// addresses on the whole stack. It is one large frame that does not fit.
//
// The reason it does not fit is the CPU. This host is a Ryzen AI MAX+
// 395 with avx512f/bw/cd/dq/ifma/vbmi/vl, so the kernel's XSAVE signal
// frame is far larger than the PAL's 16 KiB assumed:
//
//   compile-time SIGSTKSZ    = 8192      <- what the PAL was sized against
//   compile-time MINSIGSTKSZ = 2048
//   sysconf(_SC_MINSIGSTKSZ) = 3376      <- what this CPU actually needs
//
// So on this machine the overflow is DETERMINISTIC: any signal taken on
// a stock-alt-stack thread overflows by exactly 800 bytes. What is
// intermittent is only whether such a signal happens at all — which is
// why it reproduces at ~50% and why every previous hunt found it "rare".
//
// This also explains the artifacts that had gone unexplained for weeks:
// createdump produces nothing (there is no stack left to run it on), and
// the dump carries si_code=128/si_addr=0 because the kernel's
// force_sigsegv() fires when it cannot build a signal frame. That is a
// SECOND cause of the AP34 signature — the doctrine's table reads it as
// "re-raised, the registers are the handler's", and here the registers
// are the fault site. Read the mapping rsp lands in before choosing.
//
// ===================================================================
// WHY AN LD_PRELOAD AND NOT A MANAGED HOOK
// ===================================================================
//
// sigaltstack is PER-THREAD and must be called ON the thread it covers.
// CrashDiagnostics already does that for the two threads it can reach:
// the UI thread (Install() runs there) and the render thread (via
// AltStackProbe's custom draw operation). Every other thread is out of
// reach — there is no managed hook that runs on a freshly created
// thread-pool, finalizer, timer, tiered-compilation or diagnostics
// thread, and CoreCLR exposes no knob for the size (`DefaultStackSize`
// is the THREAD stack; `EnableAlternateStackCheck` is a check).
//
// Measured in the same two cores: 16 threads still carried the stock
// 16 KiB, and 3 of them had already taken a signal on it.
//
// Interposing sigaltstack catches every one of them at the exact moment
// the PAL installs its own, with no enumeration and no list to keep up
// to date. dlopen'd libraries do not interpose, so this has to be
// LD_PRELOAD rather than a constructor inside libbridge.so.
//
// ===================================================================
// CONTRACT
// ===================================================================
//
//   * Only ever ENLARGES. A caller asking for >= the floor passes
//     straight through, so nothing that already sized itself correctly
//     is second-guessed.
//   * NEVER fails a call the real one would have satisfied. If mmap or
//     the install fails we fall back to the caller's own stack, because
//     a diagnostic that breaks thread creation is worse than the bug.
//   * WB_ALTSTACK_BYTES is honoured with the SAME semantics as
//     CrashDiagnostics.AltStackWantBytes: 0 disables entirely and
//     restores the stock 16 KiB. That is the control arm, and it is the
//     only way to re-measure the bug once this is in (doctrine §5).
//   * Frees the previous replacement on re-install and at thread exit,
//     so a process that churns thread-pool threads does not leak 1 MiB
//     of address space per exited thread.
//
// The counters below are exported so the app can report what actually
// happened rather than assert that it worked. See
// CrashDiagnostics.ProbeAltStackCoverage, which measures a real thread
// instead of trusting these — a count is bookkeeping, the probe is
// evidence (D25: prefer a derived measurement to a printed count).

#define _GNU_SOURCE

#include <dlfcn.h>
#include <pthread.h>
#include <signal.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>

#ifndef MAP_STACK
#define MAP_STACK 0x20000
#endif

#define WB_ALTSTACK_DEFAULT (1UL << 20) /* 1 MiB, same default as CrashDiagnostics */

static int (*real_sigaltstack)(const stack_t *, stack_t *);

static size_t g_floor = WB_ALTSTACK_DEFAULT;
static int g_disabled;

static unsigned long g_calls;
static unsigned long g_enlarged;
static unsigned long g_failed;

/* Per-thread record of the mapping WE installed, so it can be released
   when the thread installs another one or exits. Without this the
   process leaks one floor-sized mapping per exited thread — virtual
   only (the pages commit lazily and an untouched alt stack never faults
   one in), but it would still march toward vm.max_map_count in a
   long-lived GUI that churns thread-pool threads. */
struct wb_slot {
    void *mem;
    size_t len;
};

static pthread_key_t g_key;
static int g_key_ok;

static void wb_slot_release(void *p)
{
    struct wb_slot *s = (struct wb_slot *)p;
    if (s == NULL)
        return;
    if (s->mem != NULL)
        munmap(s->mem, s->len);
    free(s);
}

__attribute__((constructor)) static void wb_altstack_init(void)
{
    real_sigaltstack = (int (*)(const stack_t *, stack_t *))dlsym(RTLD_NEXT, "sigaltstack");

    const char *env = getenv("WB_ALTSTACK_BYTES");
    if (env != NULL && *env != '\0') {
        char *end = NULL;
        long long v = strtoll(env, &end, 10);
        if (end != env) {
            if (v == 0)
                g_disabled = 1; /* the A/B control arm — stock 16 KiB */
            else if (v > 0)
                g_floor = (size_t)v;
        }
    }

    g_key_ok = (pthread_key_create(&g_key, wb_slot_release) == 0);
}

/* Exported so the frontend can report the interposer's own view. These
   are supporting detail; the load-bearing evidence is the probe thread
   in CrashDiagnostics, which measures the property directly. */
unsigned long wb_altstack_calls(void) { return __atomic_load_n(&g_calls, __ATOMIC_RELAXED); }
unsigned long wb_altstack_enlarged(void) { return __atomic_load_n(&g_enlarged, __ATOMIC_RELAXED); }
unsigned long wb_altstack_failed(void) { return __atomic_load_n(&g_failed, __ATOMIC_RELAXED); }
unsigned long wb_altstack_floor(void) { return g_disabled ? 0UL : (unsigned long)g_floor; }
/* Presence probe: a successful dlsym/DllImport of this is how a caller
   distinguishes "the interposer is loaded" from "it is not", without
   which an absent preload and a working one look identical. */
int wb_altstack_present(void) { return 1; }

int sigaltstack(const stack_t *ss, stack_t *old)
{
    if (real_sigaltstack == NULL) {
        real_sigaltstack = (int (*)(const stack_t *, stack_t *))dlsym(RTLD_NEXT, "sigaltstack");
        if (real_sigaltstack == NULL) {
            /* Nothing sane left to do; report the failure the way the
               real call would for a bad argument rather than crashing. */
            return -1;
        }
    }

    __atomic_fetch_add(&g_calls, 1, __ATOMIC_RELAXED);

    /* Pass through untouched when: disabled by the control arm; this is
       a pure query (ss == NULL); the caller is tearing the stack down;
       or the caller already asked for enough. */
    if (g_disabled || ss == NULL || (ss->ss_flags & SS_DISABLE) != 0 || ss->ss_size >= g_floor) {
        int rc = real_sigaltstack(ss, old);
        /* A genuine SS_DISABLE means our mapping is no longer installed
           and can go back now rather than at thread exit. */
        if (rc == 0 && ss != NULL && (ss->ss_flags & SS_DISABLE) != 0 && g_key_ok) {
            struct wb_slot *slot = (struct wb_slot *)pthread_getspecific(g_key);
            if (slot != NULL && slot->mem != NULL) {
                munmap(slot->mem, slot->len);
                slot->mem = NULL;
                slot->len = 0;
            }
        }
        return rc;
    }

    void *mem = mmap(NULL, g_floor, PROT_READ | PROT_WRITE,
                     MAP_PRIVATE | MAP_ANONYMOUS | MAP_STACK, -1, 0);
    if (mem == MAP_FAILED) {
        __atomic_fetch_add(&g_failed, 1, __ATOMIC_RELAXED);
        return real_sigaltstack(ss, old); /* never fail a call that would have worked */
    }

    stack_t big;
    memset(&big, 0, sizeof(big));
    big.ss_sp = mem;
    big.ss_flags = 0;
    big.ss_size = g_floor;

    int rc = real_sigaltstack(&big, old);
    if (rc != 0) {
        munmap(mem, g_floor);
        __atomic_fetch_add(&g_failed, 1, __ATOMIC_RELAXED);
        return real_sigaltstack(ss, old);
    }

    /* Installed. Release whatever we had given this thread previously
       and remember the new one for thread exit. */
    if (g_key_ok) {
        struct wb_slot *slot = (struct wb_slot *)pthread_getspecific(g_key);
        if (slot == NULL) {
            slot = (struct wb_slot *)calloc(1, sizeof(*slot));
            if (slot != NULL && pthread_setspecific(g_key, slot) != 0) {
                free(slot);
                slot = NULL;
            }
        }
        if (slot != NULL) {
            if (slot->mem != NULL)
                munmap(slot->mem, slot->len);
            slot->mem = mem;
            slot->len = g_floor;
        }
    }

    __atomic_fetch_add(&g_enlarged, 1, __ATOMIC_RELAXED);
    return rc;
}
