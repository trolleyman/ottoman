//go:build !windows

package config

import (
	"os"
	"testing"
)

func checkConfigPermissions(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("mode = %o, want 600", got)
	}
}
