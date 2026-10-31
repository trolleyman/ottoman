//go:build linux

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGreeterDeploymentAtomicallyRefreshesAndKeepsUnchangedInode(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "autostart.desktop")
	binary := filepath.Join(dir, "greeter")
	source := filepath.Join(dir, "agent")
	for path, content := range map[string]string{entry: greeterDesktopContent(), binary: "old binary", source: "new binary"} {
		if err := os.WriteFile(path, []byte(content), 0750); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := os.Stat(binary)
	installed, err := refreshGreeterInstallation(entry, binary, source)
	if err != nil || !installed {
		t.Fatal("greeter deployment failed", err)
	}
	data, err := os.ReadFile(binary)
	if err != nil || string(data) != "new binary" {
		t.Fatal("greeter did not receive deployed binary")
	}
	after, err := os.Stat(binary)
	if err != nil || os.SameFile(before, after) || after.Mode().Perm() != 0750 {
		t.Fatal("greeter replacement was not atomic/executable")
	}
	installed, err = refreshGreeterInstallation(entry, binary, source)
	if err != nil || !installed {
		t.Fatal(err)
	}
	stable, _ := os.Stat(binary)
	if !os.SameFile(after, stable) {
		t.Fatal("unchanged deploy would restart the greeter unnecessarily")
	}
}

func TestGreeterDeploymentReportsStaleOrFailedInstall(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "autostart.desktop")
	binary := filepath.Join(dir, "greeter")
	source := filepath.Join(dir, "missing-agent")
	installed, err := refreshGreeterInstallation(entry, binary, source)
	if err != nil || installed {
		t.Fatal("missing opt-in installation must be skipped")
	}
	if err = os.WriteFile(entry, []byte("outdated autostart"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = refreshGreeterInstallation(entry, binary, source); err == nil {
		t.Fatal("stale installation was reported as current")
	}
	if err = os.WriteFile(entry, []byte(greeterDesktopContent()), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = refreshGreeterInstallation(entry, binary, source); err == nil {
		t.Fatal("missing source binary was reported as deployed")
	}
	if err = os.WriteFile(source, []byte("new"), 0750); err != nil {
		t.Fatal(err)
	}
	if _, err = refreshGreeterInstallation(entry, filepath.Join(dir, "missing", "greeter"), source); err == nil {
		t.Fatal("failed greeter write was reported as deployed")
	}
}

func TestGreeterWatcherReloadsReplacementAndStopsOnCancellation(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "greeter")
	if err := os.WriteFile(binary, []byte("running"), 0750); err != nil {
		t.Fatal(err)
	}
	running, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchExecutableReplacement(ctx, binary, running, time.Millisecond, func() error { called <- struct{}{}; return nil })
	}()
	// Same-inode writes are deliberately ignored. Deployment must use rename.
	if err := os.WriteFile(binary, []byte("same inode"), 0750); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("watcher reloaded an in-place write")
	case <-time.After(10 * time.Millisecond):
	}
	if err := atomicWrite(binary, []byte("replacement"), 0750); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("running greeter did not notice deployment")
	}
	<-done
	// A canceled watcher must not trigger a later replacement.
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		watchExecutableReplacement(ctx2, binary, running, time.Hour, func() error { t.Error("reload after cancellation"); return nil })
	}()
	select {
	case <-done2:
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop")
	}
}

func TestGreeterDeviceRuleGrantsOnlyDedicatedUinputGroup(t *testing.T) {
	if !strings.Contains(uinputRuleContent, `KERNEL=="uinput"`) || !strings.Contains(uinputRuleContent, `GROUP="ottoman-input"`) || !strings.Contains(uinputRuleContent, `MODE="0660"`) || strings.Contains(uinputRuleContent, "event*") {
		t.Fatal("greeter permission rule grants more than uinput access")
	}
}
