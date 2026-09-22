#!/usr/bin/env bash
# The ONE launch path for entity-avalonia. Every make target that starts
# the app goes through this script, and that is the whole point of it.
#
# WHY IT IS NOT OPTIONAL ANY MORE (2026-09-01). Until today this script
# was copied into dist-native by `make up` alone. `make host-run` —
# reachable as `make gui-run` from the repo root, and the target our own
# docs name as "the documented fast loop" — exec'd the bare binary with
# none of these variables set, and with stderr going to a terminal
# scrollback nobody keeps.
#
# So whether a session had diagnostics depended on which target the
# operator happened to type, and nothing recorded which. That is AP41 —
# *an instrument nothing calls is indistinguishable from an instrument
# you do not have* — in its third domain, and the nastier variant,
# because AP41's check passes: something DOES call this script.
#
# Measure before you blame the launch path, though. The first write-up of
# this concluded that every operator crash had been taken unarmed. The
# crashed process's environment is inside the coredump and says
# DOTNET_DbgEnableMiniDump=1 — it was armed, and createdump did not fire
# anyway, exactly as on 2026-08-21. See the createdump note below.
#
# Everything below MUST be exported rather than merely set: the runtime
# reads them in native startup code, before any managed code exists, so a
# variable assigned from inside Main() is read too late to matter.

# ---- 1. createdump: the only artifact that yields a MANAGED stack ------
#
# A systemd-coredump ELF core cannot give one — the DAC refuses it
# (0x80004002), which is why `make crash-managed` says so out loud. Only
# createdump's own minidump carries what the DAC needs. MiniDumpType 4 is
# "full": heap included, which is what makes object inspection possible.
#
# KNOWN LIMIT, and it is the open question on this crash class: for the
# two hard SIGSEGVs we have checked, createdump was enabled and produced
# NOTHING. CrashDiagnostics' header hypothesises that by the time the
# process faults there is no stack left for createdump to run on. That is
# a hypothesis and has never been tested. Do not read a missing
# managed.*.dmp as "the variable was unset" — check the environment in
# the core before concluding anything.
export DOTNET_EnableDiagnostics=1
export DOTNET_DbgEnableMiniDump=1
export DOTNET_DbgMiniDumpType=4
# The dump goes to ~/.entity/crash/, NOT to $(pwd). pwd here is
# dist-native, and `make extract` does `rm -rf` on it — so a dump written
# beside the binary is deleted by the next build, exactly as the run log
# was. Same AP54, one file over; it survived the first pass at that fix
# because a `%d` in a path does not look like a log.
export WB_CRASH_DIR="${WB_CRASH_DIR:-$HOME/.entity/crash}"
mkdir -p "$WB_CRASH_DIR" 2>/dev/null || true
export DOTNET_DbgMiniDumpName="$WB_CRASH_DIR/managed.%d.dmp"
export DOTNET_CreateDumpDiagnostics=1

# ---- 2. perf map: names a JIT frame that otherwise reads "n/a + 0x0" ---
#
# The runtime appends every method it JITs to /tmp/perf-<pid>.map as
# `<start-hex> <size-hex> <name>`. Without it, a JIT frame in a coredump
# is an anonymous address and the only way to identify the method is to
# disassemble it by hand and infer the type from the shape of its field
# offsets. That is exactly how the 2026-09-01 crash was investigated, it
# takes hours, and it does not converge on a name.
#
# With it, `make crash` resolves the same address to a method signature.
# The cost is one append per JIT'd method at startup and a file in /tmp.
export DOTNET_PerfMapEnabled=1

# ---- 3. stderr content ------------------------------------------------
#
# Avalonia's LogToTrace() writes to System.Diagnostics.Trace, silent
# unless a listener exports it. DOTNET_LogToConsole gets core runtime
# messages onto stderr — including the runtime's own fatal-error text,
# which for a genuine managed stack overflow is a full "Stack overflow."
# report that no managed handler could ever produce.
export DOTNET_LogToConsole=1

# Breadcrumbs to stderr as well as to the durable trail file. The trail
# (~/.entity/crash/entity-avalonia-<pid>.trail) is written unconditionally
# by CrashDiagnostics and survives a signal; this is the human-readable
# copy for a session someone is watching.
export WB_PANEL_LOG=1

export LD_LIBRARY_PATH=.

echo "==> entity-avalonia diagnostics armed:"
echo "    DOTNET_DbgEnableMiniDump=$DOTNET_DbgEnableMiniDump  type=$DOTNET_DbgMiniDumpType"
echo "    DOTNET_DbgMiniDumpName=$DOTNET_DbgMiniDumpName"
echo "    WB_CRASH_DIR=$WB_CRASH_DIR  (crash log + breadcrumb trail + minidump)"
echo "    DOTNET_PerfMapEnabled=$DOTNET_PerfMapEnabled  -> /tmp/perf-<pid>.map"
echo "    PWD=$(pwd)"
echo ""

exec ./entity-avalonia "$@"
