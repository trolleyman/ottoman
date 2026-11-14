//go:build linux

package clipboard

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

func WriteImage(ctx context.Context, png []byte) error {
	var cmd *exec.Cmd
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-copy"); err != nil {
			return fmt.Errorf("image paste needs wl-clipboard installed on the desktop")
		}
		cmd = exec.CommandContext(ctx, "wl-copy", "--type", "image/png")
	} else if os.Getenv("DISPLAY") != "" {
		if _, err := exec.LookPath("xclip"); err != nil {
			return fmt.Errorf("image paste needs xclip installed on the desktop")
		}
		cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", "image/png", "-i")
	} else {
		return fmt.Errorf("image paste needs an active desktop session")
	}
	cmd.Stdin = bytes.NewReader(png)
	// These tools fork a clipboard owner. Do not attach output pipes: their
	// background child keeps them open until the selection changes.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not set the desktop image clipboard: %w", err)
	}
	return nil
}
