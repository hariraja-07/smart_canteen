package main

import "testing"

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
