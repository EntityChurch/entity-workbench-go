package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// deployment.go — the origin's own account of what it is hosting.
//
// # Why this exists
//
// An operator who has a domain and nothing else could not reach the
// registry on it. Our story was "supply the registry's peer-id out of
// band", which is correct about TRUST and useless as a first
// experience: an origin that hosts a site peer and a registry peer
// features only one of them in `{origin}/transport-profile`, so typing
// the domain into a registry browser walked the wrong peer and returned
// nothing. Reported by the operator against the live registry, in those
// words: *"I don't want to pin a key. It's an optional choice."*
//
// They were right, and the answer was already on the wire.
// `entity-deployment.json` is the cohort's deployment descriptor — the
// object `entity-browser-rust`'s browser boots from — and the live
// registry serves one carrying exactly the missing fact:
//
//	"name_registry_pin": { "origin": "...", "peer_id": "2KFNrGAR…" }
//	"home_site":         { "peer": "2KEbBKup…", "site": "…", "loc": "" }
//
// # What a pin read from here IS and IS NOT
//
// **It is trust-on-first-use, and it must be labelled that way.** An
// operator-supplied pin is the one fact in the whole chain the origin
// did not choose; a pin read from the origin is the origin nominating
// its own trust root. Everything downstream still verifies against it —
// signatures, name association, revocation — so a hostile origin cannot
// forge a binding for a key it does not hold. What it CAN do is hand you
// a key it does hold and answer every question consistently under it.
//
// So the split we hold is: **use it, and never let a surface call it a
// pin the operator made.** Refusing it instead would be the AP44 mistake
// again — protecting an invariant at the cost of the feature, when the
// honest move is to do the thing and say what it rests on.
//
// The descriptor is **not** normative. It is a cohort convention, it is
// unsigned, and it is read as a hint only. Nothing here fails a chain;
// an absent or malformed descriptor simply yields no hint.

// DeploymentFile is the object name an origin serves its deployment
// descriptor under, relative to the origin root.
const DeploymentFile = "entity-deployment.json"

// Deployment is the origin's self-description, as far as we read it.
type Deployment struct {
	// Origin is where it was read from.
	Origin string
	// RegistryPin is the registry the origin nominates, or nil. **Read
	// as a hint, never as an operator's pin** — see the file comment.
	RegistryPin *DeploymentPin
	// HomeSite is the site the deployment says it opens on. This is what
	// a site browser should default to instead of a hard-coded name.
	HomeSite DeploymentHome
}

// DeploymentPin is a nominated registry.
type DeploymentPin struct {
	Origin string
	PeerID string
}

// DeploymentHome is the deployment's front door.
type DeploymentHome struct {
	PeerID string
	Site   string
	Loc    string
}

// deploymentWire is the on-disk shape. Kept separate from the exported
// type so a field the cohort adds does not become part of our API by
// accident, and so the JSON spelling lives in exactly one place.
type deploymentWire struct {
	HomeSite struct {
		Loc  string `json:"loc"`
		Peer string `json:"peer"`
		Site string `json:"site"`
	} `json:"home_site"`
	NameRegistryPin *struct {
		Origin string `json:"origin"`
		PeerID string `json:"peer_id"`
	} `json:"name_registry_pin"`
}

// LoadDeployment fetches and decodes `{origin}/entity-deployment.json`.
//
// Every failure is reported, and every failure is survivable: callers
// treat an error as "this origin offers no hint" rather than as a fault.
// The descriptor is a convenience, and a convenience that can break a
// chain is not one.
func LoadDeployment(ctx context.Context, origin string, client *http.Client) (Deployment, error) {
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(origin, "/")
	raw, err := httpGet(ctx, client, base+"/"+DeploymentFile)
	if err != nil {
		return Deployment{Origin: base}, fmt.Errorf("fetch: deployment descriptor: %w", err)
	}
	var w deploymentWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return Deployment{Origin: base}, fmt.Errorf("fetch: decode %s: %w", DeploymentFile, err)
	}

	d := Deployment{Origin: base}
	d.HomeSite = DeploymentHome{PeerID: w.HomeSite.Peer, Site: w.HomeSite.Site, Loc: w.HomeSite.Loc}
	if p := w.NameRegistryPin; p != nil && p.PeerID != "" {
		// An empty `origin` means "the same origin this came from",
		// which is how the live deployment spells a same-host registry.
		po := strings.TrimRight(p.Origin, "/")
		if po == "" {
			po = base
		}
		d.RegistryPin = &DeploymentPin{Origin: po, PeerID: p.PeerID}
	}
	return d, nil
}
