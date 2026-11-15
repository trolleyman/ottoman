//go:build linux

package clipboard

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

func textCommand(ctx context.Context, read bool) (*exec.Cmd, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if read {
			return exec.CommandContext(ctx, "wl-paste", "--no-newline", "--type", "text"), nil
		}
		return exec.CommandContext(ctx, "wl-copy", "--type", "text/plain;charset=utf-8"), nil
	}
	if os.Getenv("DISPLAY") != "" {
		args := []string{"-selection", "clipboard", "-t", "UTF8_STRING"}
		if read {
			args = append(args, "-o")
		} else {
			args = append(args, "-i")
		}
		return exec.CommandContext(ctx, "xclip", args...), nil
	}
	return nil, errors.New("clipboard sharing needs an active desktop session")
}
func ReadText(ctx context.Context) (string, error) {
	cmd, err := textCommand(ctx, true)
	if err != nil {
		return "", err
	}
	return readCommand(ctx, cmd)
}
func WriteText(ctx context.Context, value string) error {
	cmd, err := textCommand(ctx, false)
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(value)
	return cmd.Run()
}
