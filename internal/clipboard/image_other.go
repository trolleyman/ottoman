//go:build !linux && !windows

package clipboard

import (
	"context"
	"fmt"
)

func WriteImage(context.Context, []byte) error {
	return fmt.Errorf("image paste is supported on Linux and Windows desktops")
}
