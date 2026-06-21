package entitysdk

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.entitychurch.org/entity-core-go/core/crypto"
)

// IdentityBundle is the on-disk identity material — controller
// keypair + quorum member keypairs — plus the manifest pinning the
// derived quorum-id and controller-cert-hash so cross-impl bundle
// tools (and our own re-load path) can sanity-check the
// reconstructed ceremony.
//
// On-disk layout (workbench-go shape; targets cross-impl convergence
// per SDK-IDENTITY-INFRASTRUCTURE §8.4):
//
//	~/.entity/identities/{name}/
//	├── identity.toml                        — manifest (this struct)
//	├── keypair                              — controller keypair (PEM seed)
//	└── quorum/
//	    └── members/
//	        ├── 1/keypair
//	        ├── 2/keypair
//	        └── ...
//
// Coexists with V7-only flat keypair files at
// `~/.entity/identities/{name}` (file vs directory disambiguates).
//
// Spec convergence note: §8.4 Rev 6 Amendment 1 calls for
// `identity.toml` as the manifest filename (SHOULD-tier). Our
// layout matches. We use a minimal TOML emitter — no external
// dependency — and accept fields beyond the schema for forward-
// compatibility per the same section.
type IdentityBundle struct {
	// SchemaVersion pins the on-disk format. Bump on layout changes.
	SchemaVersion string

	// Name is the bundle's directory-name. Echoed in the manifest
	// so cross-impl tools can sanity-check the bundle was loaded
	// from the expected path.
	Name string

	// CreatedAt is the unix-time the bundle was first written.
	CreatedAt int64

	// QuorumID is the content hash of the quorum entity. Pinned in
	// the manifest as a load-time integrity check — the reload path
	// re-mints the quorum entity and confirms the hash matches.
	QuorumID string // hex(33-byte content hash)

	// ControllerCertHash is the content hash of the controller-cert
	// identity attestation. Same purpose as QuorumID.
	ControllerCertHash string // hex(33-byte content hash)

	// Threshold is K in K-of-N — pinned for human-readability and
	// sanity-checking.
	Threshold int

	// QuorumName is the human-readable label that was attached to
	// the quorum entity at bootstrap time. Persisted because the
	// label participates in the canonical CBOR encoding and
	// therefore in the quorum entity's content hash — re-loading
	// with a different (or empty) name would produce a different
	// quorum-id.
	QuorumName string

	// ControllerKeypair is the controller's keypair (= the runtime
	// peer's keypair when this bundle is loaded as the local
	// identity). Held in-memory; persisted under {dir}/keypair.
	ControllerKeypair crypto.Keypair

	// QuorumMembers are the quorum constituent keypairs. Persisted
	// under {dir}/quorum/members/{i}/keypair where i is 1-indexed.
	//
	// SDK-IDENTITY-INFRASTRUCTURE §8.2 spells out the custody
	// concern: leaving these colocated with the runtime peer is the
	// catastrophic-loss surface. The minimum-viable bundle stores
	// them locally; export to separate custody is a follow-up
	// ceremony (`identity custody export ...`).
	QuorumMembers []crypto.Keypair
}

// IsIdentityBundleDir reports whether ~/.entity/identities/{name}
// is a directory bundle (identity-aware) vs a flat keypair file
// (V7-only) vs absent. Used by --identity NAME to dispatch between
// the two load paths.
func IsIdentityBundleDir(name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	dir, err := identitiesDir()
	if err != nil {
		return false, err
	}
	info, err := os.Stat(filepath.Join(dir, name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// IdentityBundleDir returns the absolute path for a named identity
// bundle (regardless of whether one exists yet).
func IdentityBundleDir(name string) (string, error) {
	dir, err := identitiesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// WriteIdentityBundle persists bundle to ~/.entity/identities/{bundle.Name}/.
// Refuses to overwrite an existing directory — callers handle
// re-bootstrap by removing the old bundle first or choosing a new
// name.
func WriteIdentityBundle(bundle IdentityBundle) error {
	if bundle.Name == "" {
		return NewError(400, "invalid_bundle", "bundle Name is required")
	}
	dir, err := IdentityBundleDir(bundle.Name)
	if err != nil {
		return WrapError(500, "home_dir_failed", "resolve identities dir", err)
	}
	if _, err := os.Stat(dir); err == nil {
		return WrapError(409, "bundle_exists",
			fmt.Sprintf("bundle %q already exists at %s", bundle.Name, dir),
			ErrIdentityExists)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return WrapError(500, "mkdir_failed", "create bundle dir", err)
	}

	if err := writeKeypairFile(filepath.Join(dir, "keypair"), bundle.ControllerKeypair); err != nil {
		return WrapError(500, "write_controller_keypair", "write controller keypair", err)
	}
	for i, m := range bundle.QuorumMembers {
		path := filepath.Join(dir, "quorum", "members", strconv.Itoa(i+1), "keypair")
		if err := writeKeypairFile(path, m); err != nil {
			return WrapError(500, "write_member_keypair",
				fmt.Sprintf("write quorum member %d", i+1), err)
		}
	}
	if err := writeBundleManifest(dir, bundle); err != nil {
		return WrapError(500, "write_manifest",
			"write identity.toml", err)
	}
	return nil
}

// LoadIdentityBundle reads a bundle from ~/.entity/identities/{name}/.
// Returns ErrIdentityNotFound (404) if the directory is absent.
func LoadIdentityBundle(name string) (IdentityBundle, error) {
	if name == "" {
		return IdentityBundle{}, NewError(400, "invalid_name", "bundle name must not be empty")
	}
	dir, err := IdentityBundleDir(name)
	if err != nil {
		return IdentityBundle{}, WrapError(500, "home_dir_failed", "resolve identities dir", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return IdentityBundle{}, WrapError(404, "bundle_not_found",
				fmt.Sprintf("no bundle at %s", dir), ErrIdentityNotFound)
		}
		return IdentityBundle{}, WrapError(500, "stat_failed", dir, err)
	}
	if !info.IsDir() {
		return IdentityBundle{}, NewError(400, "not_a_bundle",
			fmt.Sprintf("%s is a flat file (V7-only), not a directory bundle", dir))
	}

	manifest, err := readBundleManifest(dir)
	if err != nil {
		return IdentityBundle{}, WrapError(500, "read_manifest",
			"read identity.toml", err)
	}

	controllerKp, err := loadKeypairFile(filepath.Join(dir, "keypair"))
	if err != nil {
		return IdentityBundle{}, WrapError(500, "load_controller_keypair",
			"load controller keypair", err)
	}

	memberDirs, err := readMemberDirs(filepath.Join(dir, "quorum", "members"))
	if err != nil {
		return IdentityBundle{}, WrapError(500, "list_members",
			"enumerate quorum members", err)
	}
	memberKps := make([]crypto.Keypair, len(memberDirs))
	for i, mdir := range memberDirs {
		kp, err := loadKeypairFile(filepath.Join(mdir, "keypair"))
		if err != nil {
			return IdentityBundle{}, WrapError(500, "load_member_keypair",
				fmt.Sprintf("load member at %s", mdir), err)
		}
		memberKps[i] = kp
	}

	manifest.ControllerKeypair = controllerKp
	manifest.QuorumMembers = memberKps
	if manifest.Name == "" {
		manifest.Name = name
	}
	return manifest, nil
}

// writeKeypairFile writes a PEM-encoded keypair seed to path. Delegates
// PEM-encoding to crypto.SaveIdentityToDir so the header tag tracks
// kp.KeyType (BEGIN ENTITY PRIVATE KEY for Ed25519, BEGIN ENTITY ED448
// PRIVATE KEY for Ed448 — v7.67 multi-key unification).
func writeKeypairFile(path string, kp crypto.Keypair) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return crypto.SaveIdentityToDir(filepath.Dir(path), filepath.Base(path), kp)
}

// loadKeypairFile reads a PEM-encoded keypair file. Reuses
// crypto.LoadIdentityFromFile for parsing — same format as V7-only
// flat files.
func loadKeypairFile(path string) (crypto.Keypair, error) {
	return crypto.LoadIdentityFromFile(path)
}

// readMemberDirs lists the numbered subdirs under quorum/members/,
// sorted numerically (so member-1 always precedes member-10).
func readMemberDirs(parent string) ([]string, error) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, err
	}
	type entryWithIdx struct {
		idx  int
		path string
	}
	var rows []entryWithIdx
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		i, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		rows = append(rows, entryWithIdx{idx: i, path: filepath.Join(parent, e.Name())})
	}
	// Sort by index.
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].idx < rows[i].idx {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.path
	}
	return out, nil
}

// writeBundleManifest emits identity.toml. Minimal TOML emitter —
// no external dependency. Format matches what the read parser
// expects below.
func writeBundleManifest(dir string, b IdentityBundle) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# entitysdk identity bundle manifest\n"))
	sb.WriteString(fmt.Sprintf("schema_version = %q\n", b.SchemaVersion))
	sb.WriteString(fmt.Sprintf("name = %q\n", b.Name))
	sb.WriteString(fmt.Sprintf("created_at = %d\n", b.CreatedAt))
	sb.WriteString(fmt.Sprintf("quorum_id = %q\n", b.QuorumID))
	sb.WriteString(fmt.Sprintf("controller_cert_hash = %q\n", b.ControllerCertHash))
	sb.WriteString(fmt.Sprintf("threshold = %d\n", b.Threshold))
	sb.WriteString(fmt.Sprintf("member_count = %d\n", len(b.QuorumMembers)))
	sb.WriteString(fmt.Sprintf("quorum_name = %q\n", b.QuorumName))
	return os.WriteFile(filepath.Join(dir, "identity.toml"), []byte(sb.String()), 0644)
}

// readBundleManifest parses identity.toml. Minimal TOML parser —
// only the keys we emit are recognized. Unknown keys are tolerated
// (forward-compat per §8.4).
func readBundleManifest(dir string) (IdentityBundle, error) {
	data, err := os.ReadFile(filepath.Join(dir, "identity.toml"))
	if err != nil {
		return IdentityBundle{}, err
	}
	var b IdentityBundle
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		switch key {
		case "schema_version":
			b.SchemaVersion = unquote(val)
		case "name":
			b.Name = unquote(val)
		case "created_at":
			n, _ := strconv.ParseInt(val, 10, 64)
			b.CreatedAt = n
		case "quorum_id":
			b.QuorumID = unquote(val)
		case "controller_cert_hash":
			b.ControllerCertHash = unquote(val)
		case "threshold":
			n, _ := strconv.Atoi(val)
			b.Threshold = n
		case "quorum_name":
			b.QuorumName = unquote(val)
		}
	}
	return b, nil
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// hexHash is a tiny helper used during BootstrapIdentity / Load to
// stringify content hashes uniformly. Lowercase hex of the 33-byte
// wire-form per V7 §3.5.
func hexHash(b []byte) string { return hex.EncodeToString(b) }
