//go:build !linux

package ddc

import "github.com/pkg/errors"

var errUnsupported = errors.New("DDC/CI control is only supported on Linux")

// Available reports whether DDC control is available (never, off Linux).
func Available() bool { return false }

// Detect is unsupported off Linux.
func Detect() ([]Display, error) { return nil, errUnsupported }

// GetBrightness is unsupported off Linux.
func GetBrightness(bus int) (int, error) { return 0, errUnsupported }

// SetBrightness is unsupported off Linux.
func SetBrightness(bus, percent int) error { return errUnsupported }

// SetPower is unsupported off Linux.
func SetPower(bus int, on bool) error { return errUnsupported }
