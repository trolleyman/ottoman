package store

import (
	"os"
	"path/filepath"
	"strings"
)

// CurrentLayoutPath returns the path to the file recording the last-applied
// layout ID. It is historical state, not a request to apply a layout on startup.
func CurrentLayoutPath() string {
	return filepath.Join(DataDir(), "current-layout")
}

// SaveCurrentLayout records id as the last-applied layout.
func SaveCurrentLayout(id string) error {
	return writeAtomic(CurrentLayoutPath(), []byte(id+"\n"))
}

// LoadCurrentLayout returns the last-applied layout ID, or "" if none is
// recorded (or the file can't be read).
func LoadCurrentLayout() string {
	data, err := os.ReadFile(CurrentLayoutPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
