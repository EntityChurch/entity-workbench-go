using System;
using System.IO;
using System.Net;
using System.Text;
using System.Threading;

namespace EntityAvalonia.Tests;

// FixtureOrigin — a real HTTP origin serving another implementation's
// frozen bytes, with hooks a test uses to make it DISHONEST without
// editing a byte on disk.
//
// Extracted from BrowserPanelTests on 2026-08-31 because a second suite
// needed it. It was a private nested class, which is the right default
// and the wrong one the moment a fixture is worth reusing — a copy would
// have drifted from the first hostile-origin behaviour someone added to
// only one of them.
//
// The bytes stay entity-browser-rust's bytes; only what the host chooses
// to ANSWER changes, which is exactly the power EXTENSION-REGISTRY
// §6a.1a gives the party serving them (the fourth actor: it may choose
// which signed artifact answers a read, and may withhold one
// indefinitely, and may do nothing else).

internal sealed class FixtureOrigin : IDisposable
{
    // FixtureRegistryPeer is the frozen federation's registry peer-id.
    // It lives here rather than in a test class because SubstituteByName
    // has to reach into the on-disk layout, which is keyed by it.
    internal const string FixtureRegistryPeer = "2KBLkCxvkgobuauPA6zPfKarpuRRnnWHL98n8Gv1GNmybr";

    private readonly HttpListener _listener = new();
    private readonly Thread _thread;
    private readonly string _root;
    private volatile bool _stop;

    public string Prefix { get; }

    // ListingOverride replaces the served by-name listing.
    public string? ListingOverride { get; set; }

    // SubstituteByName answers the first name's by-name pointer with
    // the second name's pointer bytes.
    public (string asked, string served)? SubstituteByName { get; set; }

    // DeploymentJson, when set, is served at /entity-deployment.json —
    // the unsigned, origin-supplied hint carrying `name_registry_pin`
    // and `home_site` (AP45). An origin that serves one is nominating
    // its OWN trust root, which is why every surface that adopts a pin
    // from here owes the reader the word "trust-on-first-use".
    public string? DeploymentJson { get; set; }

    // WithholdManifest 404s the registry's published-root, which is
    // what a registry that cannot be walked looks like to a
    // consumer — and is how a test reaches the §6a.4 pointer floor.
    public bool WithholdManifest { get; set; }

    public FixtureOrigin(string root)
    {
        _root = root;
        var port = FreePort();
        Prefix = $"http://127.0.0.1:{port}";
        _listener.Prefixes.Add(Prefix + "/");
        _listener.Start();
        _thread = new Thread(Serve) { IsBackground = true };
        _thread.Start();
    }

    private static int FreePort()
    {
        var l = new System.Net.Sockets.TcpListener(IPAddress.Loopback, 0);
        l.Start();
        var port = ((IPEndPoint)l.LocalEndpoint).Port;
        l.Stop();
        return port;
    }

    private void Serve()
    {
        while (!_stop)
        {
            HttpListenerContext ctx;
            try { ctx = _listener.GetContext(); }
            catch { return; }

            try
            {
                var path = ctx.Request.Url?.AbsolutePath ?? "/";

                if (DeploymentJson != null &&
                    path.Equals("/entity-deployment.json", StringComparison.Ordinal))
                {
                    Write(ctx, Encoding.UTF8.GetBytes(DeploymentJson));
                    continue;
                }

                if (WithholdManifest && path.EndsWith("/system/peer/published-root", StringComparison.Ordinal))
                {
                    ctx.Response.StatusCode = 404;
                    ctx.Response.Close();
                    continue;
                }

                if (ListingOverride != null &&
                    path.EndsWith("/system/registry/binding/by-name.list", StringComparison.Ordinal))
                {
                    Write(ctx, Encoding.UTF8.GetBytes(ListingOverride));
                    continue;
                }

                if (SubstituteByName is { } sub &&
                    path.EndsWith("/by-name/" + sub.asked + ".bin", StringComparison.Ordinal))
                {
                    var swapped = Path.Combine(_root, "registry", FixtureRegistryPeer,
                        "system", "registry", "binding", "by-name", sub.served + ".bin");
                    Write(ctx, File.ReadAllBytes(swapped));
                    continue;
                }

                var file = Path.Combine(_root, path.TrimStart('/').Replace('/', Path.DirectorySeparatorChar));
                if (File.Exists(file))
                {
                    Write(ctx, File.ReadAllBytes(file));
                }
                else
                {
                    ctx.Response.StatusCode = 404;
                    ctx.Response.Close();
                }
            }
            catch
            {
                try { ctx.Response.Abort(); } catch { /* the client went away */ }
            }
        }
    }

    private static void Write(HttpListenerContext ctx, byte[] body)
    {
        ctx.Response.StatusCode = 200;
        ctx.Response.ContentType = "application/octet-stream";
        ctx.Response.ContentLength64 = body.Length;
        ctx.Response.OutputStream.Write(body, 0, body.Length);
        ctx.Response.Close();
    }

    public void Dispose()
    {
        _stop = true;
        try { _listener.Stop(); } catch { /* already stopped */ }
        try { _listener.Close(); } catch { /* already closed */ }
    }
}
