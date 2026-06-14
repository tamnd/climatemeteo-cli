package climatemeteo

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (resolve), which need no network. The client's
// HTTP behaviour is covered in climatemeteo_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "climatemeteo" {
		t.Errorf("Scheme = %q, want climatemeteo", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "climatemeteo" {
		t.Errorf("Identity.Binary = %q, want climatemeteo", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"52.52,13.41", "latlon", "52.52,13.41"},
		{"-33.87,151.21", "latlon", "-33.87,151.21"},
		{"2040-01-01", "date", "2040-01-01"},
		{"Berlin", "query", "Berlin"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("latlon", "52.52,13.41")
	want := "https://climate-api.open-meteo.com/v1/climate?latitude=52.52&longitude=13.41"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("page", "foo")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

// TestHostWiring mounts the driver in a kit Host and checks that the domain
// registered correctly and that ResolveOn turns a latlon string into the
// expected climatemeteo:// URI.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	// ResolveOn resolves a latlon input to a URI on the climatemeteo scheme.
	got, err := h.ResolveOn("climatemeteo", "52.52,13.41")
	if err != nil {
		t.Fatalf("ResolveOn: %v", err)
	}
	// The kit URI encodes the path, so commas become %2C.
	if want := "climatemeteo://latlon/52.52%2C13.41"; got.String() != want {
		t.Errorf("ResolveOn = %q, want %q", got.String(), want)
	}

	// The host should know the climatemeteo scheme.
	domains := h.Domains()
	found := false
	for _, d := range domains {
		if d == "climatemeteo" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("domains %v missing climatemeteo", domains)
	}
}
