// Package store handles persistence of ottoman runtime data (layouts, monitor
// registry) in the platform data directory, kept separate from the config file
// so that redeploying the config never clobbers runtime data.
package store

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// DataDir returns the ottoman data directory for the current platform:
//   - Windows: %LOCALAPPDATA%\ottoman
//   - Unix:    $XDG_DATA_HOME/ottoman (or ~/.local/share/ottoman)
//
// It falls back to the current directory if no home can be determined.
func DataDir() string {
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "ottoman")
		}
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "ottoman")
		}
		return "ottoman"
	}

	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ottoman")
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "share", "ottoman")
	}
	return "ottoman"
}

// LayoutsPath returns the path to the layouts store file.
func LayoutsPath() string {
	return filepath.Join(DataDir(), "layouts.json")
}

// RegistryPath returns the path to the monitor registry store file.
func RegistryPath() string {
	return filepath.Join(DataDir(), "registry.json")
}

// writeAtomic writes data to path via a temp file + rename so that a crash or
// concurrent read never observes a partially written file.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(err, "failed to create data directory")
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}
	tmpName := tmp.Name()

	// Best-effort cleanup if we bail before the rename.
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return errors.Wrap(err, "failed to write temp file")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return errors.Wrap(err, "failed to sync temp file")
	}
	if err := tmp.Close(); err != nil {
		return errors.Wrap(err, "failed to close temp file")
	}

	if err := os.Rename(tmpName, path); err != nil {
		return errors.Wrap(err, "failed to rename temp file into place")
	}
	tmpName = "" // rename succeeded; don't remove
	return nil
}
