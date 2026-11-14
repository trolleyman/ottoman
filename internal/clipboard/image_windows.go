//go:build windows

package clipboard

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

func WriteImage(ctx context.Context, png []byte) error {
	// The script is constant; the image travels on stdin, never in shell syntax
	// or a command argument. STA is required by the Windows clipboard API.
	const script = `$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$bytes = [Convert]::FromBase64String([Console]::In.ReadToEnd())
$stream = [IO.MemoryStream]::new($bytes, 0, $bytes.Length)
try {
 $image = [Drawing.Image]::FromStream($stream)
 try { [Windows.Forms.Clipboard]::SetImage($image) } finally { $image.Dispose() }
} finally { $stream.Dispose() }`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	cmd.Stdin = strings.NewReader(base64.StdEncoding.EncodeToString(png))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not set the desktop image clipboard: %w", err)
	}
	return nil
}
