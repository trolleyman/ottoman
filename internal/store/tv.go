package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// TVStatePath returns the path to the TV runtime-state file (holds the pairing
// key, which must survive config redeploys — hence data dir, not config).
func TVStatePath() string {
	return filepath.Join(DataDir(), "tv.json")
}

type tvState struct {
	PairingKey string `json:"pairing_key"`
}

// TVStore persists TV runtime state (the SSAP client/pairing key).
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

// LoadPairingKey reads the stored SSAP pairing key ("" if none).
func (s *TVStore) LoadPairingKey() (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Wrap(err, "failed to read TV state")
	}
	var st tvState
	if len(data) > 0 {
		if err := json.Unmarshal(data, &st); err != nil {
			return "", errors.Wrap(err, "failed to parse TV state")
		}
	}
	return st.PairingKey, nil
}

// SavePairingKey persists the SSAP pairing key atomically.
func (s *TVStore) SavePairingKey(key string) error {
	data, err := json.MarshalIndent(tvState{PairingKey: key}, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal TV state")
	}
	return writeAtomic(s.path, data)
}
