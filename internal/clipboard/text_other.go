//go:build !linux && !windows

package clipboard

import (
	"context"
	"errors"
)

func ReadText(context.Context) (string, error) {
	return "", errors.New("clipboard sharing is supported on Linux and Windows")
}
func WriteText(context.Context, string) error {
	return errors.New("clipboard sharing is supported on Linux and Windows")
}
