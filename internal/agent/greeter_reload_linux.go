//go:build linux

package agent

import (
	"context"
	"log"
	"os"
	"syscall"
	"time"
)

// The greeter has no user systemd service to restart from deployAll. Re-exec
// the new image in the existing GDM session when its installed inode changes.
// The listener and uinput descriptors are close-on-exec, so clients reconnect
// and the old virtual devices disappear without leaving keys held.
func startGreeterReload(ctx context.Context) {
	path, err := os.Executable()
	if err != nil {
		log.Printf("Greeter update watcher unavailable: %v", err)
		return
	}
	running, err := os.Stat("/proc/self/exe")
	if err != nil {
		log.Printf("Greeter update watcher unavailable: %v", err)
		return
	}
	go watchExecutableReplacement(ctx, path, running, 2*time.Second, func() error {
		log.Println("Reloading the deployed greeter agent binary")
		return syscall.Exec(path, os.Args, os.Environ())
	})
}

func watchExecutableReplacement(ctx context.Context, path string, running os.FileInfo, interval time.Duration, reexec func() error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			next, err := os.Stat(path)
			if err != nil || !next.Mode().IsRegular() || next.Mode().Perm()&0111 == 0 || os.SameFile(running, next) {
				continue
			}
			if err = reexec(); err != nil {
				log.Printf("Could not reload deployed greeter agent: %v", err)
			} else {
				return
			}
		}
	}
}
