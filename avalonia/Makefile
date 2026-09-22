# avalonia Makefile — podman-driven build, extract, and run.
#
# The build context is the parent of entity-workbench-go so the
# Containerfile can COPY both entity-core-go and entity-workbench-go
# siblings.

APP_DIR  := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
WB_DIR   := $(abspath $(APP_DIR)/..)
CTX_DIR  := $(abspath $(WB_DIR)/..)
IMAGE    := entity-avalonia:dev

# Podman resource caps — shared committed defaults live in the repo-root
# caps.mk (single source of truth, included by the root Makefile too). This
# is the heavy job implicated in the host crash, so every podman build/run
# below is fenced with $(PODMAN_BUILD_CAPS) / $(PODMAN_RUN_CAPS).
include $(WB_DIR)/caps.mk

# BUILD_STAMP — what the running app prints so you can tell two machines
# apart. `git describe` alone is not enough: during a debugging session the
# tree is dirty for hours and every build would carry the same string, which
# is exactly the situation that cost a day on 2026-09-03 (a landed fix read
# as "still broken" against a six-day-old binary). So the stamp appends a
# short hash over the working-tree diff AND the untracked/staged status, and
# a new file counts — `git diff HEAD` alone does not see one.
#
# Deterministic for an unchanged tree, so a no-op `make build` still hits
# the podman layer cache; distinct for any edit, so a rebuild always
# produces a stamp you can compare across machines.
WB_STAMP_BASE := $(shell cd $(WB_DIR) && git describe --tags --always --dirty 2>/dev/null || echo no-git)
WB_STAMP_TREE := $(shell cd $(WB_DIR) && { git diff HEAD 2>/dev/null; git status --porcelain 2>/dev/null; } | sha256sum | cut -c1-8)
BUILD_STAMP   := $(WB_STAMP_BASE)+$(WB_STAMP_TREE)

.PHONY: help build run extract host-run up test smoke-xvfb smoke-xvfb-driver smoke-xvfb-site smoke-xvfb-connections smoke-xvfb-handlers smoke-xvfb-program smoke-xvfb-window smoke-xvfb-click crash-hunt crash clean

help:
	@echo "avalonia — podman build targets for the Avalonia desktop renderer"
	@echo ""
	@echo "  make up         build + extract + launch on host (one-shot — use this)"
	@echo "  make build      build the multi-stage image ($(IMAGE))"
	@echo "  ARGS=... on up / host-run forwards app flags:"
	@echo "                  make host-run ARGS=\"--identity me --storage sqlite\""
	@echo "  make test       run headless UI tests (Avalonia.Headless.XUnit)"
	@echo "  make smoke-xvfb        run the real binary under Xvfb (real X11 paint, no display)"
	@echo "  make smoke-xvfb-driver same, but with auto-driver: ingest docs/architecture +"
	@echo "                         cycle through every doc in markdown-view"
	@echo "  make smoke-xvfb-hidpi  driver variant at 2560x1440 to catch HiDPI scaling bugs"
	@echo "  make smoke-xvfb-site   drive SITE panel: cycle Navigate across the demo site"
	@echo "  make smoke-xvfb-handlers  drive HANDLER-BROWSER panel: walk + execute every handler"
	@echo "  make smoke-xvfb-connections drive PEER-CONNECTIONS panel: pool + liveness render churn"
	@echo "  make smoke-xvfb-program PROGRAM=life|snake|asteroids|life-edit|life-big"
	@echo "                         drive the compute host panel: one panel, any program"
	@echo "  make smoke-xvfb-window  collapse/restore the WINDOW while a program paints"
	@echo "                         (MODE=resize|minimize|both) — the open-minimize-crash gate"
	@echo "  make smoke-xvfb-click  REAL X11 clicks AND DRAGS (xdotool) — the only input-path gate"
	@echo "                         CLICKS=200 CLICK_SEED=1 CLICK_GAP=120 WM=1"
	@echo "                         DRAG_PCT=40 DRAG_STEPS=12 (DRAG_PCT=0 = clicks only)"
	@echo "  make crash-hunt        run the click fuzz over HUNT_SEEDS seeds, stop at"
	@echo "                         the first crash and print the replay command"
	@echo "  make crash      decode the most recent coredump (geometry + Go frames + managed stack)"
	@echo "  make run        run the container with X11 forwarded from host"
	@echo "  make extract    copy /app out of the image to ./dist-native/"
	@echo "  make host-run   launch (extract if needed; does NOT rebuild)"
	@echo "  make clean      remove dist-native/ and the local image"
	@echo ""
	@echo "  context dir:  $(CTX_DIR)"
	@echo "  workbench:    $(WB_DIR)"
	@echo "  app:          $(APP_DIR)"

# Install our managed .containerignore at the build context root before
# every build. The canonical copy is committed inside avalonia/ so it
# travels with the repo; podman only reads .containerignore from the
# context root, hence the copy. Without this, every build invalidates
# the COPY layer (dist-native lives inside the workbench subtree).
build:
	@echo "==> building $(IMAGE) (context=$(CTX_DIR)) stamp=$(BUILD_STAMP)"
	@cp $(APP_DIR)/containerignore $(CTX_DIR)/.containerignore
	podman build $(PODMAN_BUILD_CAPS) --build-arg BUILD_STAMP=$(BUILD_STAMP) -t $(IMAGE) -f $(APP_DIR)/Containerfile $(CTX_DIR)

run:
	@echo "==> running $(IMAGE) with DISPLAY=$$DISPLAY"
	-xhost +local: 2>/dev/null || true
	podman run --rm -it $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -e DISPLAY=$$DISPLAY \
	    -e XAUTHORITY=/tmp/.Xauth \
	    -v $$XAUTHORITY:/tmp/.Xauth:ro \
	    -v /tmp/.X11-unix:/tmp/.X11-unix \
	    --net=host \
	    --name entity-avalonia-run \
	    $(IMAGE)

extract:
	@rm -rf $(APP_DIR)/dist-native
	@mkdir -p $(APP_DIR)/dist-native
	@CID=$$(podman create $(IMAGE)) && \
	    podman cp $$CID:/app/. $(APP_DIR)/dist-native/ && \
	    podman rm $$CID > /dev/null
	@echo "==> extracted to $(APP_DIR)/dist-native/"
	@echo "    run: cd $(APP_DIR)/dist-native && LD_LIBRARY_PATH=. ./entity-avalonia"

# Headless UI tests. Builds up through the `tester` stage in the
# Containerfile; that stage runs `dotnet test` against
# Workbench.Headless.Tests. A test failure fails the podman build,
# which fails this target. No image is tagged.
test:
	@echo "==> running headless UI tests (target=tester)"
	@cp $(APP_DIR)/containerignore $(CTX_DIR)/.containerignore
	@# --no-cache to force the dotnet test step to actually re-run.
	@# Build artifacts inside earlier stages are still cached via
	@# layer hashes; only the final test invocation is forced.
	podman build $(PODMAN_BUILD_CAPS) --build-arg BUILD_STAMP=$(BUILD_STAMP) --target tester --no-cache -f $(APP_DIR)/Containerfile $(CTX_DIR)

# ARGS forwards the app's own flags (--identity / --storage / --listen;
# `./entity-avalonia --help` lists them). Without it the only reachable
# configuration from a make target was the default — an ephemeral
# in-memory peer that loses everything on exit — which is a poor default
# to be *stuck* with even though it is a fine default to start from.
#
# Launches through run-with-dump.sh, same as `up`. It did NOT until
# 2026-09-01: `host-run` (= `make gui-run` at the root, the target our
# docs call the fast loop) exec'd the bare binary, so whether a session
# had createdump, a JIT symbol map or any stderr capture depended on
# which target the operator happened to type — and nothing recorded
# which, so no crash artifact could be read for it either way. The
# difference between the two targets is now the rebuild, and only the
# rebuild, and the app states its own arming at startup.
#
# Do not over-read this. It was written up first as "every operator
# crash was taken unarmed", which the coredump's own environment block
# refutes: the 2026-09-01 SIGSEGV had DOTNET_DbgEnableMiniDump=1 and
# createdump produced nothing anyway. Unarmed launches were a real hole;
# they were not the reason that crash left no managed stack.
host-run:
	@if [ ! -x $(APP_DIR)/dist-native/entity-avalonia ]; then $(MAKE) extract; fi
	@cp $(APP_DIR)/run-with-dump.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-with-dump.sh
	@mkdir -p $(APP_DIR)/run-logs
	@LOG=$(APP_DIR)/run-logs/run-$$(date +%Y%m%d-%H%M%S).log; \
	  echo "==> output -> $$LOG"; \
	  cd $(APP_DIR)/dist-native && ./run-with-dump.sh $(ARGS) 2>&1 | tee $$LOG; \
	  exit $${PIPESTATUS[0]}

# The one command you actually want during iteration: rebuild the
# container image (podman caches unchanged stages, so this is fast
# if nothing moved), re-extract dist-native, launch.
#
# stderr+stdout are tee'd to run-logs/ so crash output is captured
# every time. On non-zero exit we surface the last 40 lines AND a
# `make crash` hint.
#
# THE LOG LIVES OUTSIDE dist-native/, AND THAT IS THE WHOLE POINT.
# It was `dist-native/run.log` until 2026-09-01, and `extract` — which
# every build target runs, including `up` itself on its next invocation —
# does `rm -rf` on that directory. So the log was reliably destroyed by
# the next build. Measured: the 2026-09-01 SIGSEGV ran with
# `WB_PANEL_LOG=1`, meaning its complete breadcrumb stream up to the
# fault WAS on disk, and it was gone before anyone looked. Our own
# crash doctrine already told us not to do this — *"write artifacts
# outside dist-native/ or copy them immediately"* — and we had written
# that rule about smoke-run artifacts while the most important log in
# the repo sat in exactly the directory it names.
#
# Logs are timestamped rather than overwritten, for the same reason: the
# interesting run is usually not the most recent one.
up: build extract
	@cp $(APP_DIR)/run-with-dump.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-with-dump.sh
	@mkdir -p $(APP_DIR)/run-logs
	@LOG=$(APP_DIR)/run-logs/run-$$(date +%Y%m%d-%H%M%S).log; \
	  echo "==> launching entity-avalonia; output -> $$LOG"; \
	  cd $(APP_DIR)/dist-native && \
	  ./run-with-dump.sh $(ARGS) 2>&1 | tee $$LOG; \
	  status=$${PIPESTATUS[0]}; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> entity-avalonia exited non-zero ($$status). Last 40 log lines:"; \
	    echo "----"; tail -40 $$LOG; echo "----"; \
	    echo "==> log kept at $$LOG (outside dist-native, so the next build cannot delete it)"; \
	    echo "==> If it crashed, run \`make crash\` to decode."; \
	  fi

# Decode the most recent entity-avalonia coredump's libbridge.so
# backtrace to Go function names + file:line.  Symbols are
# kept in libbridge.so (no -s -w; see Containerfile).
crash:
	@echo "==> looking for the most recent entity-avalonia coredump..."
	@PID=$$(coredumpctl list -r --no-legend 2>/dev/null \
	    | grep entity-avalonia \
	    | head -1 \
	    | awk '{print $$5}'); \
	  if [ -z "$$PID" ]; then \
	    echo "no entity-avalonia coredump found in coredumpctl"; \
	    exit 1; \
	  fi; \
	  echo "    PID=$$PID"; echo ""; \
	  coredumpctl info $$PID 2>&1 \
	    | sed -n '/Stack trace/,/ELF object/p'; \
	  echo ""; \
	  echo "==> decoding libbridge.so addresses to Go symbols..."; \
	  coredumpctl info $$PID 2>&1 \
	    | grep -oE 'libbridge\.so \+ 0x[0-9a-f]+' \
	    | awk '{print $$3}' \
	    | while read addr; do \
	        echo -n "  $$addr -> "; \
	        addr2line -e $(APP_DIR)/dist-native/libbridge.so -f -C $$addr \
	          | tr '\n' '\t'; echo ""; \
	      done; \
	  echo ""; \
	  echo "==> triage discriminators (recorded so they are comparable run to run)"; \
	  GPUN=$$(coredumpctl info $$PID 2>/dev/null | grep -icE 'gallium|GLX_mesa|libGL\.|swrast'); \
	  BRN=$$(coredumpctl info $$PID 2>/dev/null | grep -c 'libbridge\.so'); \
	  SICODE=$$(coredumpctl info $$PID 2>/dev/null | grep -oE 'si_code: [0-9]+' | head -1); \
	  echo "    GPU/mesa modules on faulting thread : $$GPUN   (21-ish = the mesa driver bug; 0 = ours)"; \
	  echo "    libbridge.so frames                 : $$BRN   (0 = not the Go bridge)"; \
	  echo "    $${SICODE:-si_code: <not reported by coredumpctl>}"; \
	  echo "    NOTE si_code 128 (SI_KERNEL) with si_addr 0 means the signal was RE-RAISED:"; \
	  echo "         the register context is NOT the fault site, and si_addr is meaningless."; \
	  echo ""; \
	  $(MAKE) --no-print-directory crash-stack CORE_PID=$$PID; \
	  $(MAKE) crash-managed CORE_PID=$$PID

# crash-stack — the GEOMETRY half, and the one that settles "was this a
# stack overflow?" with a number instead of an impression.
#
# Added 2026-09-01 because `coredumpctl info` TRUNCATES a backtrace, and
# a truncated run of identical return addresses reads exactly like an
# unbounded recursion. It read that way once and was wrong: 45 frames, a
# uniform 448-byte stride, 19.2 KB — an ordinary tree walk. The number
# was in the core the whole time and nobody had a way to get it out.
#
# Extracts the core to a temp file (coredumpctl stores them zstd'd) and
# runs crash-stack-report.py, which also symbolizes JIT frames through
# the perf map when DOTNET_PerfMapEnabled was set at launch.
crash-stack:
	@if [ -z "$(CORE_PID)" ]; then echo "usage: make crash-stack CORE_PID=<pid>"; exit 2; fi
	@echo "==> stack geometry (is this a stack overflow, or a normal-depth walk?)"
	@TMP=$$(mktemp -d); \
	  if coredumpctl dump $(CORE_PID) --output=$$TMP/core >/dev/null 2>&1; then \
	    MAP=/tmp/perf-$(CORE_PID).map; \
	    if [ -f $$MAP ]; then \
	      python3 $(APP_DIR)/crash-stack-report.py $$TMP/core --perf-map $$MAP; \
	    else \
	      echo "    (no $$MAP — the process was launched without DOTNET_PerfMapEnabled,"; \
	      echo "     so managed frames cannot be named. run-with-dump.sh sets it, and"; \
	      echo "     every make launch target now goes through run-with-dump.sh.)"; \
	      python3 $(APP_DIR)/crash-stack-report.py $$TMP/core; \
	    fi; \
	  else \
	    echo "    could not extract the coredump for pid $(CORE_PID)"; \
	  fi; \
	  rm -rf $$TMP

# crash-managed — the managed half of the post-mortem.
#
# Two things this target exists to get right, both learned 2026-08-21:
#
#  1. **The host cannot run dotnet-dump.** dist-native/tools/dotnet-dump
#     is framework-dependent ("You must install .NET to run this
#     application"), and the dist beside it is a SELF-CONTAINED publish,
#     which is not a shared-framework layout. The old target invoked it
#     on the host and it could never have worked. Analysis runs in the
#     builder image, which has the SDK — consistent with "the host never
#     needs .NET".
#
#  2. **A systemd ELF core is not a managed dump.** The DAC refuses it
#     (`Failed to load data access module, 0x80004002`), so clrstack is
#     unavailable no matter how the tool is invoked. Only createdump's
#     own minidump carries what the DAC needs. Say that out loud instead
#     of printing a mystery hex code.
CORE_PID ?=
crash-managed:
	@DMP=$$(ls -t $${WB_CRASH_DIR:-$$HOME/.entity/crash}/managed.*.dmp \
	           $(APP_DIR)/dist-native/managed.*.dmp \
	           $(APP_DIR)/dist-native/xvfb-smoke-out*/managed.*.dmp 2>/dev/null | head -1); \
	 if [ -n "$$DMP" ]; then \
	   echo "==> analyzing managed minidump: $$DMP"; \
	   $(MAKE) --no-print-directory crash-image; \
	   podman run --rm -i $(PODMAN_RUN_CAPS) \
	     -v $$DMP:$$DMP:ro,z \
	     -v $(APP_DIR)/dist-native:$(APP_DIR)/dist-native:ro,z \
	     entity-avalonia-builder:dev \
	     /build/dist/tools/dotnet-dump analyze $$DMP \
	       -c 'clrstack -all' -c 'pe -lines' -c 'exit' 2>&1 | head -300; \
	 else \
	   echo "==> no managed minidump (managed.*.dmp) found."; \
	   echo "    run-with-dump.sh DOES export DOTNET_DbgEnableMiniDump=1, so this is not"; \
	   echo "    a missing env var — the runtime did not invoke createdump for this fault."; \
	   echo "    CONFIRM IT rather than assuming, both ways: the crashed process's whole"; \
	   echo "    environment is inside the coredump."; \
	   echo "      coredumpctl dump <pid> --output=/tmp/c && grep -ac DOTNET_DbgEnableMiniDump /tmp/c"; \
	   echo "    On 2026-09-01 that check refuted a written-up conclusion that the crash"; \
	   echo "    had been taken unarmed. It was armed. createdump simply produced nothing,"; \
	   echo "    as on 2026-08-21 — WHY is open and has never been tested."; \
	   echo "    A systemd coredump cannot substitute: the DAC rejects an ELF core, so"; \
	   echo "    there is no managed stack to be had from it. What IS available from the"; \
	   echo "    core: thread/frame layout, module list, and raw JIT disassembly via gdb."; \
	 fi

# crash-image tags the builder stage (SDK + dotnet-dump). Cached after
# the first build, so this is a no-op on a warm tree.
.PHONY: crash-image crash-managed crash-stack
crash-image:
	@cp $(APP_DIR)/containerignore $(CTX_DIR)/.containerignore
	@podman image exists entity-avalonia-builder:dev || \
	  podman build $(PODMAN_BUILD_CAPS) --target builder \
	    -t entity-avalonia-builder:dev -f $(APP_DIR)/Containerfile $(CTX_DIR)

# Xvfb smoke run. Boots the REAL binary against the REAL X11 paint
# pipeline inside a virtual framebuffer — closes the gap between
# headless tests (in-process Skia, no X11 message loop) and what
# users actually run. Captures screenshots at ~2s intervals (frames/)
# + the full stderr/stdout log. Auto-closes via WB_SMOKE_EXIT_AFTER_SEC.
#
# Default timer 15s. Override with `make smoke-xvfb SECS=30`.
#
# Idle variant (this target) just boots + renders + idles. The
# `smoke-xvfb-driver` target adds programmatic driving on top.
SECS ?= 15
smoke-xvfb: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> Xvfb smoke run (timer=$(SECS)s)"
	@cd $(APP_DIR)/dist-native && \
	  podman run --rm $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -v $(APP_DIR)/dist-native:/app:Z \
	    -w /app \
	    --entrypoint bash \
	    $(IMAGE) \
	    /app/run-xvfb-smoke.sh $(SECS); \
	  status=$$?; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> smoke-xvfb FAILED (exit $$status)"; \
	    echo "    artifacts in $(APP_DIR)/dist-native/xvfb-smoke-out/"; \
	    exit $$status; \
	  fi
	@echo "==> screenshot: $(APP_DIR)/dist-native/xvfb-smoke-out/screenshot.png"

# Driven variant — boots the app, then SmokeDriver inside the app
# ingests $(WB_DIR)/docs/architecture into the active peer, switches
# the middle panel slot to markdown-view, and cycles through every
# ingested doc N times. Reproduces the rapid-click-large-doc scenario
# the adaptive renderer was built for, under real X11 + Skia paint.
#
# Vars (override on command line):
#   SECS            run duration (sec)              default 30
#   CYCLES          publish-cycle iterations         default 60
#   GAP_MS          ms between publishes             default 150
DRIVER_SECS    ?= 30
DRIVER_CYCLES  ?= 60
DRIVER_GAP_MS  ?= 150
smoke-xvfb-driver: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> Xvfb smoke run with auto-driver (timer=$(DRIVER_SECS)s cycles=$(DRIVER_CYCLES))"
	@cd $(APP_DIR)/dist-native && \
	  podman run --rm $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -v $(APP_DIR)/dist-native:/app:Z \
	    -v $(WB_DIR)/docs:/docs:Z,ro \
	    -e WB_SMOKE_INGEST=/docs/architecture \
	    -e WB_SMOKE_CYCLE_PATHS=$(DRIVER_CYCLES) \
	    -e WB_SMOKE_CYCLE_GAP_MS=$(DRIVER_GAP_MS) \
	    -w /app \
	    --entrypoint bash \
	    $(IMAGE) \
	    /app/run-xvfb-smoke.sh $(DRIVER_SECS); \
	  status=$$?; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> smoke-xvfb-driver FAILED (exit $$status)"; \
	    echo "    artifacts in $(APP_DIR)/dist-native/xvfb-smoke-out/"; \
	    exit $$status; \
	  fi
	@echo "==> frames: $(APP_DIR)/dist-native/xvfb-smoke-out/frames/"

# HiDPI smoke — same driver harness, but at 2560x1440x24. Catches
# layout/scaling issues that don't surface at the default 1280x1024
# (e.g., text-shape edge cases, panel proportions at higher DPI).
smoke-xvfb-hidpi: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> HiDPI Xvfb smoke run (2560x1440, timer=$(DRIVER_SECS)s)"
	@cd $(APP_DIR)/dist-native && \
	  podman run --rm $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -v $(APP_DIR)/dist-native:/app:Z \
	    -v $(WB_DIR)/docs:/docs:Z,ro \
	    -e WB_SMOKE_INGEST=/docs/architecture \
	    -e WB_SMOKE_CYCLE_PATHS=$(DRIVER_CYCLES) \
	    -e WB_SMOKE_CYCLE_GAP_MS=$(DRIVER_GAP_MS) \
	    -e WB_SMOKE_SCREEN=2560x1440x24 \
	    -w /app \
	    --entrypoint bash \
	    $(IMAGE) \
	    /app/run-xvfb-smoke.sh $(DRIVER_SECS); \
	  status=$$?; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> smoke-xvfb-hidpi FAILED (exit $$status)"; \
	    echo "    artifacts in $(APP_DIR)/dist-native/xvfb-smoke-out/"; \
	    exit $$status; \
	  fi
	@echo "==> frames: $(APP_DIR)/dist-native/xvfb-smoke-out/frames/"

# SITE smoke — drives the SiteViewPanel under real X11 + Skia.
#
# The middle slot already defaults to site-view (PeerView.cs) and
# EnsureDemoSite auto-seeds the bundled demo on SiteOpen, so the
# fixture is automatic. The driver cycles Navigate across the demo's
# pages — surfaces Navigate-under-X11 regressions (paint stalls, GC
# pinning, dispatcher starvation) that headless tests miss.
#
# Tier-4 regression bar for the SITE thread (PHASE-I-SITE-VIEW-PLAN §7).
smoke-xvfb-site: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> SITE Xvfb smoke (timer=$(DRIVER_SECS)s cycles=$(DRIVER_CYCLES))"
	@cd $(APP_DIR)/dist-native && \
	  podman run --rm $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -v $(APP_DIR)/dist-native:/app:Z \
	    -e WB_SMOKE_SITE_NAVIGATE=1 \
	    -e WB_SMOKE_CYCLE_PATHS=$(DRIVER_CYCLES) \
	    -e WB_SMOKE_CYCLE_GAP_MS=$(DRIVER_GAP_MS) \
	    -w /app \
	    --entrypoint bash \
	    $(IMAGE) \
	    /app/run-xvfb-smoke.sh $(DRIVER_SECS); \
	  status=$$?; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> smoke-xvfb-site FAILED (exit $$status)"; \
	    echo "    artifacts in $(APP_DIR)/dist-native/xvfb-smoke-out/"; \
	    exit $$status; \
	  fi
	@echo "==> frames: $(APP_DIR)/dist-native/xvfb-smoke-out/frames/"

# HANDLER-BROWSER smoke — drives HandlerBrowserPanel (over
# wb.HandlerBrowserModel) under real X11: mount, walk every discovered
# handler, execute an operation on each, repaint the lists + spec line +
# growing output log each step.
#
# What this covers that headless cannot: the Skia paint path under
# repeated ItemsSource churn, and the error-rendering path — the
# executions return error statuses by design, since an arbitrary op
# dispatched with no params is exactly the case a user hits while
# exploring and exactly the one nobody tests.
# CLICK FUZZ — real X11 pointer input, the rung no other smoke target
# reaches.
#
# Every existing driver calls the model method that sits UNDER the
# control (SiteViewPanel.NavigateForTests, HandlerBrowserModel, …). That
# is a deliberate design and it is also a blind spot with a shape: it
# cannot execute input dispatch, hit-testing, focus transfer, pointer
# capture, or any handler that runs before a panel's own code. The
# 2026-08-21 SIGSEGV lived precisely there — one click on a link, and
# the fault arrived before the panel logged anything at all.
#
# CLICKS/SEED/GAP are the knobs. The seed makes a crashing run
# replayable: every click is written to clicks.log with coordinates, so
# "it crashed randomly" becomes "it crashed on click 37 at 812,455 with
# seed 4".
CLICKS      ?= 200
CLICK_SEED  ?= 1
CLICK_GAP   ?= 120
# DRAG_PCT of gestures are drags rather than clicks, half of those
# near-vertical (the scrollbar-thumb shape). Added 2026-09-01: until
# then this harness could only press and release at one point, so it
# could not take pointer capture, could not deliver a motion stream to a
# captured element, and could not move a scrollbar thumb — which is what
# the operator was doing when the process died. DRAG_PCT=0 restores the
# click-only behaviour, which is the control arm.
DRAG_PCT    ?= 40
DRAG_STEPS  ?= 12
# A window manager is ON by default here, unlike every other smoke
# target. The desktop the operator crashes on has one, and the WM owns
# focus transfer and pointer grabs — the very machinery a click fuzz is
# trying to exercise. WM= (empty) turns it off.
WM          ?= 1
# CRASH_TRACE=1 turns on first-chance exception logging for the run.
# Noisy by design — a healthy Avalonia session throws internally — but a
# managed exception thrown immediately before a hard fault is the best
# lead available when the fault itself leaves no managed dump.
CRASH_TRACE ?=
# GDB=1 runs the app under gdb and stops at the FIRST SIGSEGV, which is
# the only way to see the real fault site: the runtime re-raises, so the
# coredump's register context is the handler's, not the fault's.
GDB         ?=

smoke-xvfb-click: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> CLICK-FUZZ Xvfb smoke (timer=$(DRIVER_SECS)s clicks=$(CLICKS) seed=$(CLICK_SEED))"
	@cd $(APP_DIR)/dist-native && \
	  podman run --rm $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -v $(APP_DIR)/dist-native:/app:Z \
	    -e WB_SMOKE_CLICK_FUZZ=$(CLICKS) \
	    -e WB_SMOKE_CLICK_SEED=$(CLICK_SEED) \
	    -e WB_SMOKE_CLICK_GAP_MS=$(CLICK_GAP) \
	    -e WB_SMOKE_DRAG_PCT=$(DRAG_PCT) \
	    -e WB_SMOKE_DRAG_STEPS=$(DRAG_STEPS) \
	    -e WB_SMOKE_WM=$(WM) \
	    -e WB_CRASH_TRACE=$(CRASH_TRACE) \
	    -e WB_SMOKE_GDB=$(GDB) \
	    $(if $(GDB),--cap-add=SYS_PTRACE,) \
	    -w /app \
	    --entrypoint bash \
	    $(IMAGE) \
	    /app/run-xvfb-smoke.sh $(DRIVER_SECS); \
	  status=$$?; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> smoke-xvfb-click FAILED (exit $$status) seed=$(CLICK_SEED)"; \
	    echo "    replay: make -C avalonia smoke-xvfb-click CLICK_SEED=$(CLICK_SEED)"; \
	    echo "    artifacts in $(APP_DIR)/dist-native/xvfb-smoke-out/"; \
	    exit $$status; \
	  fi
	@echo "==> frames: $(APP_DIR)/dist-native/xvfb-smoke-out/frames/"

# CRASH-HUNT — the unattended version. Runs the click fuzz over a range
# of seeds and STOPS at the first non-zero exit, leaving that run's
# artifacts (clicks.log, crash/*.log, managed dump, frames) in place and
# printing the exact replay command.
#
# This is the target to reach for when the answer to "can you reproduce
# it?" has to be yes without a person sitting there. HUNT_SEEDS bounds
# the search; a clean sweep is a NEGATIVE result and is reported as one,
# not as "fixed".
HUNT_SEEDS ?= 12

crash-hunt: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> CRASH HUNT: $(HUNT_SEEDS) seeds x $(CLICKS) clicks, $(DRIVER_SECS)s each"
	@failed=0; \
	 for seed in $$(seq 1 $(HUNT_SEEDS)); do \
	   echo ""; echo "---- seed $$seed ----"; \
	   cd $(APP_DIR)/dist-native && \
	   podman run --rm $(PODMAN_RUN_CAPS) \
	     --userns=keep-id \
	     -v $(APP_DIR)/dist-native:/app:Z \
	     -e WB_SMOKE_CLICK_FUZZ=$(CLICKS) \
	     -e WB_SMOKE_CLICK_SEED=$$seed \
	     -e WB_SMOKE_CLICK_GAP_MS=$(CLICK_GAP) \
	     -e WB_SMOKE_DRAG_PCT=$(DRAG_PCT) \
	     -e WB_SMOKE_DRAG_STEPS=$(DRAG_STEPS) \
	     -e WB_SMOKE_WM=$(WM) \
	     -e WB_CRASH_TRACE=$(CRASH_TRACE) \
	     -e WB_SMOKE_OUT=/app/xvfb-smoke-out-seed$$seed \
	     -w /app \
	     --entrypoint bash \
	     $(IMAGE) \
	     /app/run-xvfb-smoke.sh $(DRIVER_SECS) || { \
	       echo ""; \
	       echo "==> CRASH HUNT HIT on seed $$seed"; \
	       echo "    replay: make -C avalonia smoke-xvfb-click CLICK_SEED=$$seed CLICKS=$(CLICKS)"; \
	       echo "    artifacts: $(APP_DIR)/dist-native/xvfb-smoke-out-seed$$seed/"; \
	       echo "    COPY THEM NOW — \`extract\` (which every build target runs) does"; \
	       echo "    rm -rf on dist-native, so the next make invocation deletes this."; \
	       failed=1; break; }; \
	 done; \
	 if [ $$failed -eq 0 ]; then \
	   echo ""; \
	   echo "==> crash hunt clean across $(HUNT_SEEDS) seeds."; \
	   echo "    This is a NEGATIVE result, not a fix: it bounds the search, it does not close it."; \
	 fi; \
	 exit $$failed

smoke-xvfb-handlers: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> HANDLER-BROWSER Xvfb smoke (timer=$(DRIVER_SECS)s walk=$(DRIVER_CYCLES))"
	@cd $(APP_DIR)/dist-native && \
	  podman run --rm $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -v $(APP_DIR)/dist-native:/app:Z \
	    -e WB_SMOKE_HANDLERS=1 \
	    -e WB_SMOKE_CYCLE_PATHS=$(DRIVER_CYCLES) \
	    -e WB_SMOKE_CYCLE_GAP_MS=$(DRIVER_GAP_MS) \
	    -w /app \
	    --entrypoint bash \
	    $(IMAGE) \
	    /app/run-xvfb-smoke.sh $(DRIVER_SECS); \
	  status=$$?; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> smoke-xvfb-handlers FAILED (exit $$status)"; \
	    echo "    artifacts in $(APP_DIR)/dist-native/xvfb-smoke-out/"; \
	    exit $$status; \
	  fi
	@echo "==> frames: $(APP_DIR)/dist-native/xvfb-smoke-out/frames/"

# PEER-CONNECTIONS — drives the panel carrying both connection surfaces
# (the local pool AND the tree's liveness record) under real X11 + Skia.
# The liveness half landed 2026-08-20; this is its tier-3 gate.
#
# Read the log, not just the exit code: this harness runs ONE peer, so
# no lifecycle transition is ever written and the liveness list is
# legitimately empty. The run gates mount, handle allocation and render
# churn — the AP24 class — not row content, and it says so in run.log.
smoke-xvfb-connections: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> PEER-CONNECTIONS Xvfb smoke (timer=$(DRIVER_SECS)s cycles=$(DRIVER_CYCLES))"
	@cd $(APP_DIR)/dist-native && \
	  podman run --rm $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -v $(APP_DIR)/dist-native:/app:Z \
	    -e WB_SMOKE_CONNECTIONS=1 \
	    -e WB_SMOKE_CYCLE_PATHS=$(DRIVER_CYCLES) \
	    -e WB_SMOKE_CYCLE_GAP_MS=$(DRIVER_GAP_MS) \
	    -w /app \
	    --entrypoint bash \
	    $(IMAGE) \
	    /app/run-xvfb-smoke.sh $(DRIVER_SECS); \
	  status=$$?; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> smoke-xvfb-connections FAILED (exit $$status)"; \
	    echo "    artifacts in $(APP_DIR)/dist-native/xvfb-smoke-out/"; \
	    exit $$status; \
	  fi
	@echo "==> frames: $(APP_DIR)/dist-native/xvfb-smoke-out/frames/"

# GENERIC HOST — drives ProgramPanel for any of the three programs through the
# ONE generic panel, under real X11 + Skia.
#
#   make smoke-xvfb-program PROGRAM=life
#   make smoke-xvfb-program PROGRAM=snake
#   make smoke-xvfb-program PROGRAM=asteroids
#
# The point is what does NOT vary: one panel class, one driver, one accessor,
# every program mounted from a descriptor. If the shape vocabulary were secretly
# game-shaped, one of the three would need special handling here.
#
# This target used to have three per-program siblings (smoke-xvfb-life / -snake
# / -asteroids) driving the legacy panels, kept so the two paths could be
# compared live. Both the panels and their drivers were retired 2026-08-20; the
# comparison is done and the equality is pinned by frozen vectors instead
# (programs/oracle_vectors_test.go).
PROGRAM ?= life

smoke-xvfb-program: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> GENERIC-HOST Xvfb smoke: program=$(PROGRAM) (timer=$(DRIVER_SECS)s samples=$(DRIVER_CYCLES))"
	@cd $(APP_DIR)/dist-native && \
	  podman run --rm $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -v $(APP_DIR)/dist-native:/app:Z \
	    -e WB_SMOKE_PROGRAM=$(PROGRAM) \
	    -e WB_SMOKE_CYCLE_PATHS=$(DRIVER_CYCLES) \
	    -e WB_SMOKE_CYCLE_GAP_MS=$(DRIVER_GAP_MS) \
	    -w /app \
	    --entrypoint bash \
	    $(IMAGE) \
	    /app/run-xvfb-smoke.sh $(DRIVER_SECS); \
	  status=$$?; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> smoke-xvfb-program ($(PROGRAM)) FAILED (exit $$status)"; \
	    echo "    artifacts in $(APP_DIR)/dist-native/xvfb-smoke-out/"; \
	    exit $$status; \
	  fi
	@echo "==> frames: $(APP_DIR)/dist-native/xvfb-smoke-out/frames/"

# WINDOW — collapse and restore the window itself while a program paints.
# The one gate in this repo that can reach the open managed stack overflow:
# it fires on minimize on a real desktop, its two documented predecessors
# were a GridSplitter driving a row to zero height, and the headless suite
# survives all three of its collapse repros because headless never runs the
# X11 backend. Xvfb does.
#
#   MODE=resize    (default) collapse ClientSize toward zero and restore.
#                  A resize is an X ConfigureWindow, honored with no WM.
#   MODE=minimize  cycle WindowState. READ THE LOG — iconify is a window-
#                  MANAGER operation and this harness runs a bare Xvfb with
#                  no WM, so it may be a no-op. The driver logs the state it
#                  actually observed and says so when nothing moved.
#   MODE=both      alternate.
#
# PROGRAM= layers a live repaint stream underneath (the crash needs paint in
# flight, not an idle window). Set PROGRAM= empty for the idle site-view case.
WINDOW_MODE    ?= resize
WINDOW_PROGRAM ?= life
# WM=1 starts openbox on the virtual display first. REQUIRED for
# MODE=minimize to mean anything — iconify is a window-manager operation,
# and with no WM the driver correctly reports zero transitions.
WINDOW_WM      ?=
smoke-xvfb-window: build extract
	@cp $(APP_DIR)/run-xvfb-smoke.sh $(APP_DIR)/dist-native/
	@chmod +x $(APP_DIR)/dist-native/run-xvfb-smoke.sh
	@echo "==> WINDOW Xvfb smoke: mode=$(WINDOW_MODE) under program=$(WINDOW_PROGRAM) (timer=$(DRIVER_SECS)s cycles=$(DRIVER_CYCLES))"
	@cd $(APP_DIR)/dist-native && \
	  podman run --rm $(PODMAN_RUN_CAPS) \
	    --userns=keep-id \
	    -v $(APP_DIR)/dist-native:/app:Z \
	    -e WB_SMOKE_WINDOW=$(WINDOW_MODE) \
	    $(if $(WINDOW_WM),-e WB_SMOKE_WM=$(WINDOW_WM),) \
	    $(if $(WINDOW_PROGRAM),-e WB_SMOKE_PROGRAM=$(WINDOW_PROGRAM),) \
	    -e WB_SMOKE_CYCLE_PATHS=$(DRIVER_CYCLES) \
	    -e WB_SMOKE_CYCLE_GAP_MS=$(DRIVER_GAP_MS) \
	    -w /app \
	    --entrypoint bash \
	    $(IMAGE) \
	    /app/run-xvfb-smoke.sh $(DRIVER_SECS); \
	  status=$$?; \
	  if [ $$status -ne 0 ]; then \
	    echo ""; \
	    echo "==> smoke-xvfb-window FAILED (exit $$status) — if this is a SIGSEGV or a"; \
	    echo "    stack overflow, that is the OPEN BUG REPRODUCING. Capture the dump"; \
	    echo "    before it rotates: make crash"; \
	    echo "    artifacts in $(APP_DIR)/dist-native/xvfb-smoke-out/"; \
	    exit $$status; \
	  fi
	@echo "==> window cycle survived; grep run.log for 'WINDOW' to see what actually moved"
	@echo "    $(APP_DIR)/dist-native/xvfb-smoke-out/run.log"

clean:
	rm -rf $(APP_DIR)/dist-native
	-podman rmi $(IMAGE) 2>/dev/null || true
