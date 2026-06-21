package shellcmd

import (
	"encoding/json"
	"fmt"
)

// cmdPut implements `put <path> <type> <json-data>`. The data
// argument is parsed as JSON; if it fails to parse, the literal
// string is stored. The path follows shell resolution rules
// (peer-qualified or alias:relative).
func cmdPut(sh *Shell, args []string) (Result, error) {
	if len(args) < 3 {
		return Result{}, fmt.Errorf("usage: put <path> <type> <json-data>")
	}
	target := sh.Resolve(args[0])
	typeName := args[1]
	rawData := args[2]

	if target.IsRoot() {
		return Result{}, fmt.Errorf("cannot put at root (cd into a peer first)")
	}
	pc := sh.ConnForPath(target)
	if pc == nil {
		return Result{}, fmt.Errorf("no connection for path %s", target)
	}

	var data interface{}
	if err := json.Unmarshal([]byte(rawData), &data); err != nil {
		// Not valid JSON — treat as literal string.
		data = rawData
	}

	h, err := pc.Peer.Put(target.String(), typeName, data)
	if err != nil {
		return Result{}, fmt.Errorf("put: %w", err)
	}
	return MessageResult(fmt.Sprintf("put %s [%s] → %s", target, typeName, h.String())), nil
}

// cmdRm implements `rm <path>`.
func cmdRm(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: rm <path>")
	}
	target := sh.Resolve(args[0])
	if target.IsRoot() {
		return Result{}, fmt.Errorf("cannot rm root")
	}
	pc := sh.ConnForPath(target)
	if pc == nil {
		return Result{}, fmt.Errorf("no connection for path %s", target)
	}
	if err := pc.Peer.Remove(target.String()); err != nil {
		return Result{}, fmt.Errorf("rm: %w", err)
	}
	return MessageResult(fmt.Sprintf("removed %s", target)), nil
}

// cmdHas implements `has <path>`. Reports yes/no via a Result
// message; returns an error only on dispatch failure (404 is
// reported as no, not an error).
func cmdHas(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: has <path>")
	}
	target := sh.Resolve(args[0])
	if target.IsRoot() {
		return Result{}, fmt.Errorf("has requires a peer path")
	}
	pc := sh.ConnForPath(target)
	if pc == nil {
		return Result{}, fmt.Errorf("no connection for path %s", target)
	}
	ok, err := pc.Peer.Has(target.String())
	if err != nil {
		return Result{}, fmt.Errorf("has: %w", err)
	}
	if ok {
		return MessageResult(fmt.Sprintf("yes — %s exists", target)), nil
	}
	return MessageResult(fmt.Sprintf("no — %s does not exist", target)), nil
}

// cmdCp implements `cp <src> <dst>`. Reads the entity at src and
// writes it verbatim at dst (preserving content hash). src and dst
// can target different peers — cross-peer copy works because both
// dispatch through the local AppPeer's pool.
func cmdCp(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: cp <src> <dst>")
	}
	src := sh.Resolve(args[0])
	dst := sh.Resolve(args[1])

	if src.IsRoot() || dst.IsRoot() {
		return Result{}, fmt.Errorf("cp requires non-root paths on both sides")
	}

	srcPC := sh.ConnForPath(src)
	if srcPC == nil {
		return Result{}, fmt.Errorf("no connection for src %s", src)
	}
	dstPC := sh.ConnForPath(dst)
	if dstPC == nil {
		return Result{}, fmt.Errorf("no connection for dst %s", dst)
	}

	ent, ok, err := srcPC.Peer.Get(src.String())
	if err != nil {
		return Result{}, fmt.Errorf("get src: %w", err)
	}
	if !ok {
		return Result{}, fmt.Errorf("no entity at src %s", src)
	}

	h, err := dstPC.Peer.PutEntity(dst.String(), ent)
	if err != nil {
		return Result{}, fmt.Errorf("put dst: %w", err)
	}
	return MessageResult(fmt.Sprintf("copied %s → %s [%s] (%s)", src, dst, ent.Type, h.String())), nil
}
