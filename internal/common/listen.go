package common

import (
	"errors"
	"log"
	"net"
	"strings"
	"syscall"
	"time"
)

// How long to keep retrying a bind that fails only because the address is still
// in use, and how long to wait between attempts.
const (
	listenRetryWindow   = 20 * time.Second
	listenRetryInterval = 500 * time.Millisecond
)

// ListenWithRetry binds addr, retrying while the address is still in use.
//
// At login the previous session's instance can still hold the port for a few
// seconds while it shuts down. A single attempt fails, the process exits, and
// the service manager restarts it — it recovers, but the restart is noise and
// leaves a window where the server is unreachable. Waiting for the old process
// to let go is quicker and quieter. Any other error fails immediately, so a
// genuinely misconfigured address still surfaces at once.
func ListenWithRetry(network, addr string) (net.Listener, error) {
	deadline := time.Now().Add(listenRetryWindow)
	warned := false
	for {
		ln, err := net.Listen(network, addr)
		if err == nil {
			return ln, nil
		}
		if !isAddrInUse(err) || !time.Now().Before(deadline) {
			return nil, err
		}
		if !warned {
			log.Printf("Address %s is in use (likely a previous instance shutting down); retrying for up to %s", addr, listenRetryWindow)
			warned = true
		}
		time.Sleep(listenRetryInterval)
	}
}

// isAddrInUse reports whether a listen error means the address is already bound.
// Windows reports this as WSAEADDRINUSE with different wording, so the message
// is checked as well as the errno.
func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}
