package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These need no database, and must keep running when there is none.

func TestListenPortDefaultsToEightyEighty(t *testing.T) {
	t.Setenv("PORT", "")
	got, err := listenPort()
	if err != nil {
		t.Fatalf("PORT unset: %v, want 8080 with no error", err)
	}
	if got != "8080" {
		t.Errorf("PORT unset: got %q, want 8080", got)
	}
}

func TestListenPortHonoursTheHost(t *testing.T) {
	t.Setenv("PORT", " 10000 ")
	got, err := listenPort()
	if err != nil {
		t.Fatalf("PORT=10000: %v", err)
	}
	if got != "10000" {
		t.Errorf("PORT=10000: got %q, want 10000", got)
	}
}

// A bad PORT must be an error, never a silent fallback to 8080. Falling back
// would leave the service listening on a port the host is not sending traffic
// to, which is the failure this function exists to prevent.
func TestListenPortRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"http", "80.5", "-1", "0", "70000", "8080a"} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("PORT", bad)
			got, err := listenPort()
			if err == nil {
				t.Errorf("PORT=%q was accepted as %q, want an error", bad, got)
			}
			if got != "" {
				t.Errorf("PORT=%q: got port %q alongside the error, want none", bad, got)
			}
		})
	}
}

// A whitespace-only PORT is treated as unset rather than as garbage: some
// shells export an empty-looking variable, and failing to start over that would
// be a hostile surprise.
func TestListenPortTreatsBlankAsUnset(t *testing.T) {
	t.Setenv("PORT", "   ")
	got, err := listenPort()
	if err != nil || got != "8080" {
		t.Errorf("PORT=whitespace: got (%q, %v), want (8080, nil)", got, err)
	}
}

// --- CORS ---

func TestCORSOriginDefaultsToWildcard(t *testing.T) {
	// Unset and blank both keep the behaviour the API has always had, so
	// existing deployments do not start rejecting their own clients.
	for _, unset := range []string{"", "   ", "*", " * "} {
		t.Run("CORS_ALLOWED_ORIGIN="+unset, func(t *testing.T) {
			t.Setenv("CORS_ALLOWED_ORIGIN", unset)
			if got := corsOrigin(); got != "*" {
				t.Errorf("got %q, want *", got)
			}
		})
	}
}

func TestCORSOriginUsesAConfiguredValue(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGIN", "  https://canteen.example  ")
	if got := corsOrigin(); got != "https://canteen.example" {
		t.Errorf("got %q, want the trimmed origin", got)
	}
}

// The header has to reach a real client over a real socket, since that is the
// only place a browser decides whether to let a page read a response. Asserted
// through httptest.NewServer rather than newRouter called directly, so a
// middleware that only works in one arrangement cannot pass this.
func TestCORSHeaderOnARealRequest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		set    string
		want   string
		absent bool
	}{
		{name: "unset allows any origin", set: "", want: "*"},
		{name: "a named origin is sent verbatim", set: "https://canteen.example",
			want: "https://canteen.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CORS_ALLOWED_ORIGIN", tc.set)
			srv := newTestCORSServer(t)
			resp, err := http.Get(srv + "/health")
			if err != nil {
				t.Fatalf("GET /health: %v", err)
			}
			defer resp.Body.Close()
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != tc.want {
				t.Errorf("Allow-Origin: got %q, want %q", got, tc.want)
			}
			if got := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
				t.Errorf("Allow-Headers: got %q, want it to include Authorization", got)
			}
		})
	}
}

// A preflight is a browser asking permission before it will send the real
// request. Answering it 404 or 405 makes the browser give up on the whole call
// even though the actual endpoint would have worked.
func TestCORSPreflightIsAnswered(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGIN", "https://canteen.example")
	srv := newTestCORSServer(t)

	req, err := http.NewRequest(http.MethodOptions, srv+"/api/orders", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", "https://canteen.example")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("preflight: got %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://canteen.example" {
		t.Errorf("preflight Allow-Origin: got %q, want the configured origin", got)
	}
}

// newTestCORSServer serves the real middleware over a socket. The /health route
// needs a database, so a router with no routes is used instead: this is about
// the wrapper, not what is behind it.
func newTestCORSServer(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(withCORS(mux))
	t.Cleanup(srv.Close)
	return srv.URL
}
