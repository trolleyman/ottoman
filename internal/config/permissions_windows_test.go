package config

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// Windows files inherit their directory ACL. Compare the config with a fresh
// file in the same private directory, since Stat cannot report Windows ACLs.
func checkConfigPermissions(t *testing.T, path string) {
	t.Helper()
	reference := filepath.Join(filepath.Dir(path), "acl-reference")
	if err := os.WriteFile(reference, nil, 0600); err != nil {
		t.Fatal(err)
	}
	descriptor := func(name string) string {
		sd, err := windows.GetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		return sd.String()
	}
	if descriptor(path) != descriptor(reference) {
		t.Errorf("config permissions differ from directory-inherited permissions")
	}
}
