package agent

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
	gnomeext "github.com/trolleyman/ottoman/gnome-extension"
)

// installGnomeExtension writes the embedded GNOME Shell Quick Settings
// extension into the user's extensions directory and best-effort enables it.
// It is idempotent (overwrites on every install). On Wayland the shell only
// discovers a newly-installed extension after a log out/in, so enabling may not
// take effect until then; failures are non-fatal and reported to the caller.
func installGnomeExtension() error {
	home := os.Getenv("HOME")
	if home == "" {
		return errors.New("HOME environment variable must be set")
	}

	dst := filepath.Join(home, ".local", "share", "gnome-shell", "extensions", gnomeext.UUID)
	if err := os.MkdirAll(dst, 0755); err != nil {
		return errors.Wrap(err, "failed to create extension directory")
	}

	entries, err := fs.ReadDir(gnomeext.Files(), ".")
	if err != nil {
		return errors.Wrap(err, "failed to read embedded extension")
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(gnomeext.Files(), e.Name())
		if err != nil {
			return errors.Wrapf(err, "failed to read embedded %s", e.Name())
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0644); err != nil {
			return errors.Wrapf(err, "failed to write %s", e.Name())
		}
	}

	fmt.Printf("Installed GNOME extension to %s\n", dst)

	// Best-effort enable. On Wayland the running shell hasn't rescanned the
	// extensions dir yet, so this often reports "extension does not exist" until
	// the next login — hence the log-out/in hint below regardless of outcome.
	if _, err := exec.LookPath("gnome-extensions"); err == nil {
		if out, err := exec.Command("gnome-extensions", "enable", gnomeext.UUID).CombinedOutput(); err != nil {
			fmt.Printf("  (could not enable yet: %s)\n", string(out))
		} else {
			fmt.Println("  Enabled.")
		}
	}
	fmt.Println("  Log out and back in (Wayland) for it to appear in Quick Settings,")
	fmt.Printf("  then if needed: gnome-extensions enable %s\n", gnomeext.UUID)
	return nil
}
