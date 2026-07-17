package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// TVStatePath returns the path to the TV runtime-state file (holds the pairing
// keys, which must survive config redeploys — hence data dir, not config).
func TVStatePath() string {
	return filepath.Join(DataDir(), "tv.json")
}

type tvState struct {
	// PairingKey is the legacy single-TV key from before TVs were keyed by
	// monitor EDID. It is kept as a fallback so an existing install's TV stays
	// paired, and is superseded by a PairingKeys entry once one is saved.
	PairingKey  string            `json:"pairing_key,omitempty"`
	PairingKeys map[string]string `json:"pairing_keys,omitempty"` // by monitor EDID
}

// TVStore persists TV runtime state (the SSAP client/pairing keys).
type TVStore struct {
	path string
}

// NewTVStore returns a store backed by the given path (default TVStatePath if empty).
func NewTVStore(path string) *TVStore {
	if path == "" {
		path = TVStatePath()
	}
	return &TVStore{path: path}
}

func (s *TVStore) load() (tvState, error) {
	var st tvState
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, errors.Wrap(err, "failed to read TV state")
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &st); err != nil {
			return st, errors.Wrap(err, "failed to parse TV state")
		}
	}
	return st, nil
}

// LoadPairingKey reads the stored SSAP pairing key for a TV monitor ("" if
// none). A monitor without its own key falls back to the legacy single-TV key.
func (s *TVStore) LoadPairingKey(edid string) (string, error) {
	st, err := s.load()
	if err != nil {
		return "", err
	}
	if key, ok := st.PairingKeys[edid]; ok && key != "" {
		return key, nil
	}
	return st.PairingKey, nil
}

// SavePairingKey persists a TV monitor's SSAP pairing key atomically,
// preserving other monitors' keys.
func (s *TVStore) SavePairingKey(edid, key string) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	if st.PairingKeys == nil {
		st.PairingKeys = make(map[string]string)
	}
	st.PairingKeys[edid] = key
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal TV state")
	}
	return writeAtomic(s.path, data)
}
