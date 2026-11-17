//go:build windows

package clipboard

import (
	"context"
	"encoding/base64"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func ReadText(ctx context.Context) (string, error) {
	const script = `$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms; [Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); [Console]::Write([Windows.Forms.Clipboard]::GetText())`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	return readCommand(ctx, cmd)
}
func WriteText(ctx context.Context, value string) error {
	const script = `$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms; $value=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String([Console]::In.ReadToEnd())); if ($value.Length -eq 0) { [Windows.Forms.Clipboard]::Clear() } else { [Windows.Forms.Clipboard]::SetText($value) }`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	cmd.Stdin = strings.NewReader(base64.StdEncoding.EncodeToString([]byte(value)))
	return cmd.Run()
}
