// Throwaway smoke test for libbridge.so — validates all three POC
// spikes from plain C, so the bridge half is verified independently
// of Avalonia.
//
// Build + run on host:
//   gcc bridge_smoke.c -o /tmp/smoke -ldl
//   cd avalonia-poc/dist-native && LD_LIBRARY_PATH=. /tmp/smoke
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <unistd.h>
#include <dlfcn.h>

typedef char*   (*init_fn)(void);
typedef char*   (*hello_fn)(void);
typedef char*   (*dispatch_fn)(const char*);
typedef int64_t (*watch_sub_fn)(const char*, void*);
typedef void    (*watch_unsub_fn)(int64_t);
typedef void    (*free_fn)(char*);
typedef void    (*shutdown_fn)(void);

static int watch_count = 0;

static void on_watch_event(int64_t handle, const char* json) {
    watch_count++;
    printf("  [watch handle=%lld] %s\n", (long long)handle, json);
}

int main(int argc, char** argv) {
    void* lib = dlopen("./libbridge.so", RTLD_NOW);
    if (!lib) { fprintf(stderr, "dlopen: %s\n", dlerror()); return 1; }

    init_fn       Init     = (init_fn)       dlsym(lib, "BridgeInit");
    hello_fn      Hello    = (hello_fn)      dlsym(lib, "Hello");
    dispatch_fn   Dispatch = (dispatch_fn)   dlsym(lib, "Dispatch");
    watch_sub_fn  WSub     = (watch_sub_fn)  dlsym(lib, "WatchSubscribe");
    watch_unsub_fn WUnsub  = (watch_unsub_fn) dlsym(lib, "WatchUnsubscribe");
    free_fn       Free     = (free_fn)       dlsym(lib, "FreeString");
    shutdown_fn   Shutdown = (shutdown_fn)   dlsym(lib, "BridgeShutdown");

    if (!Init || !Hello || !Dispatch || !WSub || !WUnsub || !Free || !Shutdown) {
        fprintf(stderr, "dlsym failed\n"); return 1;
    }

    printf("=== Spike 1: Hello round-trip ===\n");
    char* err = Init();
    if (err) { fprintf(stderr, "BridgeInit error: %s\n", err); Free(err); return 1; }
    printf("  BridgeInit ok\n");
    char* greeting = Hello();
    printf("  Hello: %s\n", greeting);
    Free(greeting);

    printf("\n=== Spike 3: shellcmd dispatch ===\n");
    char* pwd = Dispatch("[\"pwd\"]");           printf("  pwd: %s\n", pwd); Free(pwd);
    char* ls  = Dispatch("[\"ls\"]");            printf("  ls:  %s\n", ls);  Free(ls);
    char* cd  = Dispatch("[\"cd\",\"@poc\"]");   printf("  cd @poc: %s\n", cd); Free(cd);
    char* pwd2 = Dispatch("[\"pwd\"]");          printf("  pwd: %s\n", pwd2); Free(pwd2);

    printf("\n=== Spike 2: Store.Watch via C callback ===\n");
    int64_t h = WSub("demo/*", &on_watch_event);
    printf("  WatchSubscribe handle=%lld\n", (long long)h);
    if (h < 0) { fprintf(stderr, "watch subscribe failed\n"); return 1; }

    // Trigger 3 puts; expect 3 callback invocations.
    const char* puts[] = {
        "[\"put\",\"demo/a\",\"test/x\",\"\\\"hello\\\"\"]",
        "[\"put\",\"demo/b\",\"test/x\",\"42\"]",
        "[\"put\",\"demo/c\",\"test/x\",\"true\"]",
    };
    for (int i = 0; i < 3; i++) {
        char* r = Dispatch(puts[i]);
        printf("  put %d: %s\n", i, r);
        Free(r);
    }
    // Give the watch fanout goroutine a beat to fire the callbacks.
    usleep(200 * 1000);
    printf("  callbacks observed: %d (expected 3)\n", watch_count);

    WUnsub(h);
    printf("  WatchUnsubscribe ok\n");

    Shutdown();
    printf("\nShutdown ok\n");
    return watch_count == 3 ? 0 : 1;
}
