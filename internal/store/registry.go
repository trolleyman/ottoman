package store

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/pkg/errors"
)

// Control backend identifiers for a monitor.
const (
	BackendNone = "none" // no external control
	BackendDDC  = "ddc"  // DDC/CI over i2c (desktop monitors)
	BackendTV   = "tv"   // network API (e.g. LG webOS)
)

// Control names used in visibility overrides.
const (
	ControlBrightness = "brightness"
	ControlPower      = "power"
	ControlVolume     = "volume"
)

// MonitorEntry is the persisted registry record for one physical monitor,
// keyed by its stable EDID identifier.
type MonitorEntry struct {
	Edid string `json:"edid"`
	// FriendlyName overrides the display name in UIs (empty = use detected name).
	FriendlyName string `json:"friendly_name,omitempty"`
	// Backend is the control backend: "ddc", "tv", or "none".
	Backend string `json:"backend,omitempty"`
	// Visibility maps a control name (brightness/power/volume) to whether it
	// should be shown. A control absent from the map defaults to visible.
	Visibility map[string]bool `json:"visibility,omitempty"`
}

// Visible reports whether a control should be shown for this monitor.
func (e MonitorEntry) Visible(control string) bool {
	if e.Visibility == nil {
		return true
	}
	v, ok := e.Visibility[control]
	if !ok {
		return true
	}
	return v
}

// Registry is a persisted, EDID-keyed store of monitor settings.
type Registry struct {
	path string
	mu   sync.Mutex
	// entries is keyed by EDID.
	entries map[string]MonitorEntry
}

type registryFile struct {
	Monitors []MonitorEntry `json:"monitors"`
}

// NewRegistry returns a registry backed by the given path (default RegistryPath
// if empty). It loads existing entries from disk.
func NewRegistry(path string) (*Registry, error) {
	if path == "" {
		path = RegistryPath()
	}
	r := &Registry{path: path, entries: make(map[string]MonitorEntry)}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Registry) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "failed to read registry")
	}
	if len(data) == 0 {
		return nil
	}
	var f registryFile
	if err := json.Unmarshal(data, &f); err != nil {
		return errors.Wrap(err, "failed to parse registry")
	}
	for _, e := range f.Monitors {
		if e.Edid != "" {
			r.entries[e.Edid] = e
		}
	}
	return nil
}

// save writes the registry atomically. Caller must hold r.mu.
func (r *Registry) save() error {
	f := registryFile{Monitors: make([]MonitorEntry, 0, len(r.entries))}
	for _, e := range r.entries {
		f.Monitors = append(f.Monitors, e)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal registry")
	}
	return writeAtomic(r.path, data)
}

// Get returns the entry for an EDID, and whether it existed.
func (r *Registry) Get(edid string) (MonitorEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[edid]
	return e, ok
}

// Ensure returns the entry for an EDID, creating a default one (with the given
// backend) and persisting it if it did not exist yet.
func (r *Registry) Ensure(edid, defaultBackend string) (MonitorEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[edid]; ok {
		return e, nil
	}
	e := MonitorEntry{Edid: edid, Backend: defaultBackend}
	r.entries[edid] = e
	if err := r.save(); err != nil {
		return e, err
	}
	return e, nil
}

// Update applies a mutation to an entry (creating it if missing) and persists.
func (r *Registry) Update(edid string, fn func(*MonitorEntry)) (MonitorEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[edid]
	e.Edid = edid
	fn(&e)
	r.entries[edid] = e
	if err := r.save(); err != nil {
		return e, err
	}
	return e, nil
}

// List returns all entries.
func (r *Registry) List() []MonitorEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]MonitorEntry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}
