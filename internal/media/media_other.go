//go:build !linux

package media

import (
	"context"
	"errors"
)

func Read(context.Context) (State, error) { return State{}, nil }
func Control(context.Context, string, string) error {
	return errors.New("now-playing control is unavailable on this desktop")
}
