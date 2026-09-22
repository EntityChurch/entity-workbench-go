package fetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"entity-workbench-go/fetch"
)

// deployment_test.go — the origin's self-description.
//
// These exist because an operator with a domain and nothing else could
// not reach the registry on it, and the missing fact was already on the
// wire. The live registry's descriptor is transcribed verbatim below;
// if the cohort's spelling moves, this is where it fails.
//
// Tier: contract pin.

const liveDescriptor = `{
  "home_site": {
    "loc": "",
    "peer": "2KEbBKupL9RZsV4Jx8zYvK6g4EnikvCPSZL2zPEafybVkR",
    "site": "entity-church-registry-main"
  },
  "name_registry_pin": {
    "origin": "https://entitychurchregistry.org",
    "peer_id": "2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb"
  },
  "origins": { "2KEbBKupL9RZsV4Jx8zYvK6g4EnikvCPSZL2zPEafybVkR": "" },
  "site_mode": { "enabled": false, "show_toggle": false },
  "surface": "window",
  "window_type": "Site Browser"
}`

func serve(t *testing.T, path, body string) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/"+path, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestDeploymentCarriesTheRegistryPinAndHomeSite is the whole point: the
// two facts an operator would otherwise have to be told out of band.
func TestDeploymentCarriesTheRegistryPinAndHomeSite(t *testing.T) {
	origin := serve(t, fetch.DeploymentFile, liveDescriptor)

	d, err := fetch.LoadDeployment(context.Background(), origin, nil)
	if err != nil {
		t.Fatalf("LoadDeployment: %v", err)
	}
	if d.RegistryPin == nil {
		t.Fatal("no registry pin — the operator is back to typing a peer-id")
	}
	if got, want := d.RegistryPin.PeerID, "2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb"; got != want {
		t.Errorf("pin peer: got %s, want %s", got, want)
	}
	if got, want := d.RegistryPin.Origin, "https://entitychurchregistry.org"; got != want {
		t.Errorf("pin origin: got %s, want %s", got, want)
	}
	if got, want := d.HomeSite.Site, "entity-church-registry-main"; got != want {
		t.Errorf("home site: got %s, want %s", got, want)
	}
}

// TestDeploymentEmptyPinOriginMeansThisOrigin — the live descriptors use
// "" for a same-host registry, and reading that as an empty origin
// produces a request to nowhere.
func TestDeploymentEmptyPinOriginMeansThisOrigin(t *testing.T) {
	origin := serve(t, fetch.DeploymentFile,
		`{"name_registry_pin":{"origin":"","peer_id":"2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb"}}`)

	d, err := fetch.LoadDeployment(context.Background(), origin, nil)
	if err != nil {
		t.Fatalf("LoadDeployment: %v", err)
	}
	if d.RegistryPin == nil || d.RegistryPin.Origin != origin {
		t.Fatalf("empty pin origin did not resolve to the serving origin: %+v", d.RegistryPin)
	}
}

// TestDeploymentAbsenceIsSurvivable — the descriptor is a convenience,
// and a convenience that breaks a chain is not one. Every one of these
// must yield "no hint", never a fault the caller has to handle.
func TestDeploymentAbsenceIsSurvivable(t *testing.T) {
	for _, tc := range []struct{ name, path, body string }{
		{"no descriptor served", "something-else", "{}"},
		{"malformed json", fetch.DeploymentFile, "{not json"},
		{"no pin field", fetch.DeploymentFile, `{"home_site":{"site":"x"}}`},
		{"pin with empty peer_id", fetch.DeploymentFile, `{"name_registry_pin":{"peer_id":""}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origin := serve(t, tc.path, tc.body)
			d, err := fetch.LoadDeployment(context.Background(), origin, nil)
			// An error is fine; a pin that is not there is the contract.
			if err == nil && d.RegistryPin != nil {
				t.Errorf("invented a pin from %q: %+v", tc.body, d.RegistryPin)
			}
		})
	}
}
