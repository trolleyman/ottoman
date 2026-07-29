package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
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
//
// Waiting only helps if whatever holds the port is about to let go. Before
// settling in to retry, listenConflictHint looks for a holder that never will;
// if it finds one the error comes back at once, with the fix in it.
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
			if hint := listenConflictHint(addr); hint != "" {
				return nil, fmt.Errorf("%w\n%s", err, hint)
			}
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

// listenConflictHint explains an address-in-use failure whose cause is standing
// configuration rather than a previous instance still shutting down. It returns
// "" when there is nothing to say, in which case retrying is the right move.
//
// The case it catches: a `tailscale serve` mapping on the same port. tailscaled
// binds that port on the machine's tailnet address, so a wildcard bind here
// collides with it - and unlike a departing instance, the mapping is permanent,
// so retrying for the full window and then dying teaches you nothing. Binding
// loopback instead is the fix: that is where the serve mapping forwards anyway,
// and it keeps the port off every other interface.
func listenConflictHint(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return ""
	}
	// Only the wildcard case. A bind to one specific address that a serve
	// mapping doesn't cover has some other cause, and a confidently wrong
	// diagnosis is worse than none.
	if !isWildcardHost(host) || !tailscaleServesPort(port) {
		return ""
	}
	return fmt.Sprintf("`tailscale serve` is already listening on port %[1]s, so binding it on the wildcard address can never succeed. "+
		"Bind loopback instead - that is where the serve mapping forwards to:\n"+
		"    ottoman config set agent.listen_address 127.0.0.1:%[1]s        (or controller.listen_address)\n"+
		"    ottoman config set agent.require_local_auth true               (the front-end dials in from 127.0.0.1, so the loopback exemption has to go)", port)
}

// isWildcardHost reports whether host means "every interface".
func isWildcardHost(host string) bool {
	if host == "" || host == "*" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsUnspecified()
}

// serveStatus is the slice of `tailscale serve status --json` this needs: TCP
// is keyed by port, Web by "host:port".
type serveStatus struct {
	TCP map[string]json.RawMessage
	Web map[string]json.RawMessage
}

// tailscaleServeStatus reads the serve config; a var so tests can stand in for
// the CLI.
var tailscaleServeStatus = readTailscaleServeStatus

// tailscaleServesPort reports whether tailscaled holds a serve mapping on port.
func tailscaleServesPort(port string) bool {
	raw, ok := tailscaleServeStatus()
	if !ok {
		return false
	}
	var status serveStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return false
	}
	if _, ok := status.TCP[port]; ok {
		return true
	}
	for hostPort := range status.Web {
		if _, p, err := net.SplitHostPort(hostPort); err == nil && p == port {
			return true
		}
	}
	return false
}

// readTailscaleServeStatus runs the CLI, reporting false if tailscale isn't
// installed or doesn't answer promptly. This sits on the startup path, so it
// must never be the thing that hangs it.
func readTailscaleServeStatus() ([]byte, bool) {
	bin, err := exec.LookPath("tailscale")
	if err != nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "serve", "status", "--json").Output()
	if err != nil {
		return nil, false
	}
	return out, true
}
