package shellcmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// cmdRole dispatches the `role <subcommand> [args...]` command surface
// against the local peer's role extension. Per SHELL-DIRECTION.md
// §5.3 — role management lives at the shell level on top of
// entitysdk's RoleClient.
//
// Subcommands:
//   - role list <context>                                — list role definitions in a context
//   - role define <context> <name> <handler:op:resource>...  — define a role with grants
//   - role assign <context> <peer-hash-hex> <name>       — assign a peer to a role
//   - role unassign <context> <peer-hash-hex> [name]     — remove assignment(s)
//   - role exclude <context> <peer-hash-hex>             — exclude a peer (sweeps caps)
//   - role unexclude <context> <peer-hash-hex>           — remove exclusion entity
//   - role re-derive <context> <name>                    — re-issue caps after definition change
func cmdRole(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: role <list|define|assign|unassign|exclude|unexclude|re-derive> [args]")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "define":
		return cmdRoleDefine(sh, rest)
	case "assign":
		return cmdRoleAssign(sh, rest)
	case "unassign":
		return cmdRoleUnassign(sh, rest)
	case "exclude":
		return cmdRoleExclude(sh, rest)
	case "unexclude":
		return cmdRoleUnexclude(sh, rest)
	case "re-derive", "rederive":
		return cmdRoleReDerive(sh, rest)
	default:
		return Result{}, fmt.Errorf("unknown role subcommand: %s", sub)
	}
}

// cmdRoleDefine: role define <context> <name> <grant>...
//
// Grant string format is `handler:op[,op...]:resource[,resource...]`,
// e.g. `system/tree:get,list:public/*,knowledge/*`. Multiple grants
// can be passed as separate args. This is a deliberately terse form
// for shell ergonomics; structured input lands when we have a
// structured renderer-input pipeline.
func cmdRoleDefine(sh *Shell, args []string) (Result, error) {
	if len(args) < 3 {
		return Result{}, fmt.Errorf("usage: role define <context> <name> <handler:ops:resources>...")
	}
	contextName, roleName := args[0], args[1]
	var grants []types.GrantEntry
	for _, raw := range args[2:] {
		ge, err := parseGrantSpec(raw)
		if err != nil {
			return Result{}, fmt.Errorf("invalid grant %q: %w", raw, err)
		}
		grants = append(grants, ge)
	}
	if len(grants) == 0 {
		return Result{}, fmt.Errorf("role define: at least one grant required")
	}
	rc := sh.Local.Peer.Role()
	res, err := rc.Define(context.Background(), contextName, roleName, grants, nil)
	if err != nil {
		return Result{}, fmt.Errorf("role define: %w", err)
	}
	msg := fmt.Sprintf("defined role %q in context %q at %s", roleName, contextName, res.RolePath)
	if res.ReDerivedCount > 0 {
		msg += fmt.Sprintf(" (re-derived %d existing assignments)", res.ReDerivedCount)
	}
	return MessageResult(msg), nil
}

func cmdRoleAssign(sh *Shell, args []string) (Result, error) {
	if len(args) < 3 {
		return Result{}, fmt.Errorf("usage: role assign <context> <peer-hash-hex> <name>")
	}
	contextName, peerHashHex, roleName := args[0], args[1], args[2]
	peerHash, err := parsePeerHashHex(peerHashHex)
	if err != nil {
		return Result{}, fmt.Errorf("peer-hash: %w", err)
	}
	rc := sh.Local.Peer.Role()
	res, err := rc.Assign(context.Background(), contextName, peerHash, roleName)
	if err != nil {
		return Result{}, fmt.Errorf("role assign: %w", err)
	}
	return MessageResult(fmt.Sprintf("assigned %s to %q in %q (caps issued: %d)",
		shortHash(peerHash), roleName, contextName, len(res.DerivedTokens))), nil
}

func cmdRoleUnassign(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: role unassign <context> <peer-hash-hex> [name]")
	}
	contextName, peerHashHex := args[0], args[1]
	roleName := "" // empty means all-roles form
	if len(args) >= 3 {
		roleName = args[2]
	}
	peerHash, err := parsePeerHashHex(peerHashHex)
	if err != nil {
		return Result{}, fmt.Errorf("peer-hash: %w", err)
	}
	rc := sh.Local.Peer.Role()
	res, err := rc.Unassign(context.Background(), contextName, peerHash, roleName)
	if err != nil {
		return Result{}, fmt.Errorf("role unassign: %w", err)
	}
	return MessageResult(fmt.Sprintf("unassigned %s in %q (revoked %d cap(s))",
		shortHash(peerHash), contextName, len(res.RevokedTokenHashes))), nil
}

func cmdRoleExclude(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: role exclude <context> <peer-hash-hex>")
	}
	contextName, peerHashHex := args[0], args[1]
	peerHash, err := parsePeerHashHex(peerHashHex)
	if err != nil {
		return Result{}, fmt.Errorf("peer-hash: %w", err)
	}
	rc := sh.Local.Peer.Role()
	res, err := rc.Exclude(context.Background(), contextName, peerHash)
	if err != nil {
		return Result{}, fmt.Errorf("role exclude: %w", err)
	}
	return MessageResult(fmt.Sprintf("excluded %s in %q (swept %d cap(s))",
		shortHash(peerHash), contextName, len(res.RevokedTokenHashes))), nil
}

func cmdRoleUnexclude(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: role unexclude <context> <peer-hash-hex>")
	}
	contextName, peerHashHex := args[0], args[1]
	peerHash, err := parsePeerHashHex(peerHashHex)
	if err != nil {
		return Result{}, fmt.Errorf("peer-hash: %w", err)
	}
	rc := sh.Local.Peer.Role()
	if _, err := rc.Unexclude(context.Background(), contextName, peerHash); err != nil {
		return Result{}, fmt.Errorf("role unexclude: %w", err)
	}
	return MessageResult(fmt.Sprintf(
		"unexcluded %s in %q (re-assignment required to restore caps)",
		shortHash(peerHash), contextName)), nil
}

func cmdRoleReDerive(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: role re-derive <context> <name>")
	}
	contextName, roleName := args[0], args[1]
	rc := sh.Local.Peer.Role()
	res, err := rc.ReDerive(context.Background(), contextName, roleName)
	if err != nil {
		return Result{}, fmt.Errorf("role re-derive: %w", err)
	}
	msg := fmt.Sprintf("re-derived %d cap(s) for role %q in %q",
		res.ReDerivedCount, roleName, contextName)
	if len(res.SkippedGrantees) > 0 {
		msg += fmt.Sprintf(" (skipped %d due to RL2)", len(res.SkippedGrantees))
	}
	return MessageResult(msg), nil
}

// parseGrantSpec parses the shell-friendly "handler:ops:resources" form.
// Multiple ops or resources are comma-separated.
//
// Examples:
//
//	system/tree:get,list:public/*
//	system/tree:put:scratch/*
//	system/role:*:system/role/*
func parseGrantSpec(raw string) (types.GrantEntry, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return types.GrantEntry{},
			fmt.Errorf("expected handler:ops:resources, got %d colon-separated parts", len(parts))
	}
	handler, ops, resources := parts[0], parts[1], parts[2]
	if handler == "" || ops == "" || resources == "" {
		return types.GrantEntry{},
			fmt.Errorf("all three parts (handler, ops, resources) must be non-empty")
	}
	return types.GrantEntry{
		Handlers:   types.CapabilityScope{Include: []string{handler}},
		Operations: types.CapabilityScope{Include: strings.Split(ops, ",")},
		Resources:  types.CapabilityScope{Include: strings.Split(resources, ",")},
	}, nil
}

// parsePeerHashHex accepts the lowercase hex of a 33-byte system/hash
// (algorithm byte + 32-byte digest), as emitted by core-go's
// ext/role.HashHex. Per V7 §3.5 — hash segments use the full
// type-prefixed form.
func parsePeerHashHex(s string) (hash.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return hash.Hash{}, fmt.Errorf("not valid hex: %w", err)
	}
	h, err := hash.FromBytes(b)
	if err != nil {
		return hash.Hash{}, err
	}
	return h, nil
}

// shortHash returns a 12-char prefix of the hex form, useful for log lines.
func shortHash(h hash.Hash) string {
	full := hex.EncodeToString(h.Bytes())
	if len(full) > 12 {
		return full[:12] + "..."
	}
	return full
}
