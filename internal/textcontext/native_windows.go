//go:build windows

package textcontext

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"os/exec"
	"time"
	"unicode/utf16"
)

// The helper uses the UI Automation assemblies shipped with Windows. Text never
// enters process arguments, a script file, stderr, or the clipboard.
//
//go:embed windows.ps1
var windowsHelper string

type automation struct {
	window int
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
}

func Native(ctx context.Context) (Backend, error) {
	units := utf16.Encode([]rune(windowsHelper))
	data := make([]byte, len(units)*2)
	for i, v := range units {
		binary.LittleEndian.PutUint16(data[i*2:], v)
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		return nil, err
	}
	a := &automation{cmd: cmd, in: in, out: bufio.NewReader(out)}
	ready := make(chan error, 1)
	go func() {
		line, err := a.out.ReadString('\n')
		if err == nil && line != "ready\r\n" && line != "ready\n" {
			err = ErrUnavailable
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			a.Close()
			return nil, err
		}
		return a, nil
	case <-ctx.Done():
		a.Close()
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		a.Close()
		return nil, ErrUnavailable
	}
}
func (a *automation) Close() error {
	a.in.Close()
	if a.cmd.Process != nil {
		a.cmd.Process.Kill()
	}
	return a.cmd.Wait()
}
func (a *automation) request(ctx context.Context, v interface{}, out interface{}) error {
	// Cancellation interrupts a stalled third-party accessibility provider.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			a.cmd.Process.Kill()
		case <-done:
		}
	}()
	defer close(done)
	if err := json.NewEncoder(a.in).Encode(v); err != nil {
		return err
	}
	line, err := a.out.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, out)
}
func (a *automation) Snapshot(ctx context.Context, window int) (Snapshot, error) {
	a.window = window
	var result Snapshot
	err := a.request(ctx, map[string]interface{}{"op": "read", "window": window}, &result)
	return result, err
}
func (a *automation) Replace(ctx context.Context, s Snapshot, e Edit) error {
	var result struct{ OK bool }
	err := a.request(ctx, map[string]interface{}{"op": "edit", "window": a.window, "expected": s, "edit": e}, &result)
	if err != nil {
		return err
	}
	if !result.OK {
		return ErrUnavailable
	}
	return nil
}
