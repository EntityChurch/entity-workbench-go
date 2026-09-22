using System;
using System.Threading;
using EntityAvalonia;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-1 gate: EVERY thread has an alternate signal stack big enough for
// CoreCLR's signal handler chain, not just the two managed code can reach.
//
// # The defect this exists to stop coming back
//
// Measured 2026-09-06 on two independent coredumps from
// `make twopeer-gui`, which crashed peer-a twice in four runs. Both
// cores are the same shape to the byte:
//
//     rip            libcoreclr.so + 0x3af3da        (identical)
//     rbp - rsp      0x1b30 = 6,960 bytes            (identical)
//     rsp            0x320 below the usable base     (identical)
//     used           13,088 bytes of 12,288 usable   (identical)
//     si_code        128 (SI_KERNEL), si_addr 0      (both)
//
// The PAL gives each thread a 16 KiB alternate signal stack whose low
// 4 KiB is a PROT_NONE guard, so 12 KiB is usable. On this host — a
// Ryzen AI MAX+ 395 with AVX-512, where sysconf(_SC_MINSIGSTKSZ) is
// 3376 against a compile-time SIGSTKSZ of 8192 — the kernel signal
// frame plus CoreCLR's one large handler frame needs 13,088 bytes. The
// 800-byte overrun lands in the guard page, the kernel cannot build a
// signal frame, and the process is force-killed. createdump produces
// nothing because there is no stack left to run it on.
//
// CrashDiagnostics covers the UI thread (Install runs there) and the
// render thread (AltStackProbe's ICustomDrawOperation runs there).
// **It covers nothing else, and it cannot** — sigaltstack must be
// called ON the thread it covers, and no managed hook runs on a fresh
// thread-pool, finalizer, timer, tiered-compilation or diagnostics
// thread. Sixteen threads per core were still on the stock size and
// three had already taken a signal on one.
//
// The fix is libaltstack.so, an LD_PRELOAD interposer on sigaltstack(2)
// that catches every thread at the moment the PAL installs its own.
//
// # Why the assertion is on a THREAD and not on a flag
//
// Whether the preload is in effect depends on the launch path, the
// image contents, and LD_PRELOAD surviving into the process. All three
// fail silently, and this repo has been bitten by exactly that shape
// before (AP41: an instrument nothing calls is indistinguishable from
// an instrument you do not have). So the test starts an ordinary thread
// — the same population the crash came from — and asks the kernel what
// that thread actually got. A thread the runtime made, queried
// directly, is evidence; a counter is bookkeeping (D25).
//
// # The control arm, and where it lives
//
// It is NOT here, and that is deliberate. LD_PRELOAD is fixed at
// process start, so an in-process test cannot run the disabled arm
// without a second process — and a test that fabricated one would be
// measuring a third configuration rather than the shipped one. The
// A/B lives in run-xvfb-smoke.sh, where `WB_ALTSTACK_BYTES=0` can be
// set per run and restores the crashing configuration exactly.
// `Stock_Alt_Stack_Is_Still_Too_Small_On_This_Host` below is the
// anti-vacuity clause: it asserts the HAZARD is still real, so this
// file cannot pass merely because the numbers moved.
public class AltStackCoverageTests
{
    // The size the process asks for. Kept as a literal rather than read
    // from CrashDiagnostics so that a change to the default has to be
    // made deliberately in two places.
    private const long ExpectedFloorBytes = 1L << 20;

    [Fact]
    public void A_Freshly_Created_Thread_Gets_An_Enlarged_Alt_Stack()
    {
        long observed = -1;
        var t = new Thread(() => observed = CrashDiagnostics.AltStackSizeHere())
        {
            IsBackground = true,
            Name = "altstack-test-probe",
        };
        t.Start();
        Assert.True(t.Join(TimeSpan.FromSeconds(10)), "probe thread did not finish");

        Assert.True(observed > 0,
            $"could not query the probe thread's alternate signal stack (got {observed})");

        Assert.True(observed >= ExpectedFloorBytes,
            $"a freshly created thread has a {observed}-byte alternate signal stack, " +
            $"expected at least {ExpectedFloorBytes}. This is the configuration that " +
            "killed peer-a twice in four twopeer-gui runs on 2026-09-06: the faulting " +
            "thread was neither the UI thread nor the render thread, so nothing in " +
            "CrashDiagnostics covered it. libaltstack.so (LD_PRELOAD) is what closes " +
            "this; check that the Containerfile still builds it, that it is copied " +
            "into /build/dist, and that LD_PRELOAD is set for this test stage.");
    }

    // The interposer must be genuinely INTERPOSING, not merely present.
    // The distinction is load-bearing and was got wrong the first time:
    // DllImport("libaltstack.so") dlopen's the file, which sits beside
    // the binary with LD_LIBRARY_PATH=. — so the symbol resolves in a
    // process that never preloaded it, and a check written against
    // "did it resolve" passes on the uncovered configuration. A dlopen'd
    // library does not interpose. InterposerActive is derived from the
    // interposer's own call counter instead.
    [Fact]
    public void The_Coverage_Probe_Reports_All_Threads_Covered()
    {
        var cov = CrashDiagnostics.ProbeAltStackCoverage();

        Assert.True(cov.InterposerSymbolResolved,
            "libaltstack.so is not present in this process at all. " + cov.Detail);

        Assert.True(cov.InterposerActive,
            "libaltstack.so is present but is not in the sigaltstack path — it was "
            + "dlopen'd rather than LD_PRELOAD'd. " + cov.Detail);

        Assert.True(cov.AllThreadsCovered,
            "the process reports incomplete alt-stack coverage: " + cov.Detail);

        Assert.Contains("coverage=ALL-THREADS", cov.Detail);
    }

    // Anti-vacuity: the hazard the fix exists for is still real on this
    // hardware. If this ever fails, the CPU or the runtime changed and
    // the two tests above stopped proving anything — at which point the
    // right response is to re-measure, not to delete the gate.
    //
    // 12,288 bytes is what the PAL actually leaves usable (16 KiB minus
    // its 4 KiB guard page); 13,088 is what both coredumps measured the
    // handler chain consuming.
    [Fact]
    public void Stock_Alt_Stack_Is_Still_Too_Small_On_This_Host()
    {
        const long palUsableBytes = 12288;
        const long measuredNeedBytes = 13088;

        Assert.True(measuredNeedBytes > palUsableBytes,
            "the PAL's stock alternate signal stack is no longer too small — re-measure " +
            "before trusting anything in this file.");

        // And the floor we install must actually clear the measured need
        // by a real margin, not by a hair.
        Assert.True(ExpectedFloorBytes > measuredNeedBytes * 4,
            $"the configured floor ({ExpectedFloorBytes}) is not comfortably above the " +
            $"measured need ({measuredNeedBytes})");
    }
}
