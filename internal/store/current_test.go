package store

import "testing"

func TestCurrentLayoutRoundTrip(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if got := LoadCurrentLayout(); got != "" {
		t.Fatalf("expected empty current layout initially, got %q", got)
	}

	if err := SaveCurrentLayout("with-tv"); err != nil {
		t.Fatalf("SaveCurrentLayout: %v", err)
	}
	if got := LoadCurrentLayout(); got != "with-tv" {
		t.Fatalf("LoadCurrentLayout = %q, want %q", got, "with-tv")
	}

	// Overwrite is honoured and trailing newline is trimmed.
	if err := SaveCurrentLayout("normal"); err != nil {
		t.Fatalf("SaveCurrentLayout: %v", err)
	}
	if got := LoadCurrentLayout(); got != "normal" {
		t.Fatalf("LoadCurrentLayout after overwrite = %q, want %q", got, "normal")
	}
}
