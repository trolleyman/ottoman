//go:build !linux

package audio

import "github.com/pkg/errors"

func newPlatformController() (Controller, error) {
	return nil, errors.New("audio control is not yet supported on this platform")
}
