package shellcmd

import (
	"fmt"
	"sort"
)

// cmdInfo implements `info [alias]`. Returns one or more PeerInfo
// payloads depending on context:
//   - With explicit alias: that peer.
//   - Inside a peer's tree (working directory): that peer.
//   - At root with no args: all connected peers.
func cmdInfo(sh *Shell, args []string) (Result, error) {
	if len(args) > 0 {
		alias := args[0]
		pc, ok := sh.Conns[alias]
		if !ok {
			return Result{}, fmt.Errorf("not connected: %s", alias)
		}
		return Result{Kind: KindInfo, Info: peerInfoFor(pc)}, nil
	}
	if pc := sh.ConnForWD(); pc != nil {
		return Result{Kind: KindInfo, Info: peerInfoFor(pc)}, nil
	}
	if len(sh.Conns) == 0 {
		return MessageResult("no connections"), nil
	}
	// Multiple peers: pre-format into Lines for v1 simplicity. A future
	// pass can introduce a KindInfoList variant if richer presentation
	// is needed.
	aliases := make([]string, 0, len(sh.Conns))
	for a := range sh.Conns {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	var lines []string
	for _, a := range aliases {
		pc := sh.Conns[a]
		lines = append(lines, formatPeerInfo(pc)...)
		lines = append(lines, "")
	}
	return LinesResult(lines), nil
}

func peerInfoFor(pc *PeerConn) *PeerInfo {
	addr := pc.Address
	if addr == "" {
		addr = "(self)"
	}
	return &PeerInfo{
		Alias:   pc.Alias,
		Address: addr,
		PeerID:  pc.PeerID,
	}
}

func formatPeerInfo(pc *PeerConn) []string {
	addr := pc.Address
	if addr == "" {
		addr = "(self)"
	}
	return []string{
		fmt.Sprintf("Alias:   %s", pc.Alias),
		fmt.Sprintf("Address: %s", addr),
		fmt.Sprintf("PeerID:  %s", pc.PeerID),
	}
}

// cmdHelp implements `help [command]`.
func cmdHelp(sh *Shell, args []string) (Result, error) {
	r := Default()
	if len(args) > 0 {
		name := args[0]
		c := r.Lookup(name)
		if c == nil {
			return Result{}, fmt.Errorf("unknown command: %s", name)
		}
		return LinesResult([]string{
			"  " + c.Usage,
			"    " + c.Help,
		}), nil
	}
	lines := []string{"Commands:"}
	for _, c := range r.Commands() {
		lines = append(lines, fmt.Sprintf("  %-35s %s", c.Usage, c.Help))
	}
	lines = append(lines, "",
		"Path forms (any command that takes a path accepts these):",
		"  /                          Shell root — lists connected peers.",
		"  @alias                     Peer's root (shorthand for /@alias/).",
		"  @alias/foo/bar             Path inside a peer via its alias.",
		"  /@alias/foo/bar            Same, canonical absolute form.",
		"  /{peerID}/foo/bar          Same, with the literal peer-id.",
		"  foo/bar                    Relative — only after `cd @alias`.",
		"  ..                         Parent.",
		"  alias:... (deprecated)     Old form, still accepted — use @alias instead.",
		"",
		"Typical flow:",
		"  ls                          → see peers (incl. 'local')",
		"  cd @local                   → enter your local peer",
		"  ls                          → list its tree",
		"  put scratch/x text/v '\"hi\"'  → write something",
		"  cat scratch/x               → read it back",
		"",
		"Connecting to a remote:",
		"  connect serv 127.0.0.1:9002",
		"  cd @serv",
		"  ls",
	)
	return LinesResult(lines), nil
}
