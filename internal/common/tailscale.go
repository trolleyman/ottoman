package common

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// serveStatus is the slice of `tailscale serve status --json` this package
// needs. TCP is keyed by the port tailscaled listens on, Web by "host:port",
// and AllowFunnel by the same "host:port" as Web.
type serveStatus struct {
	TCP         map[string]tcpHandler
	Web         map[string]webConfig
	AllowFunnel map[string]bool
}

type tcpHandler struct {
	TCPForward string
}

type webConfig struct {
	Handlers map[string]webHandler
}

type webHandler struct {
	Proxy string
}

// tailscaleServeStatus reads the serve config; a var so tests can stand in for
// the CLI.
var tailscaleServeStatus = readTailscaleServeStatus

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

// readServeStatus parses the serve config, or false if there is none to read.
func readServeStatus() (serveStatus, bool) {
	raw, ok := tailscaleServeStatus()
	if !ok {
		return serveStatus{}, false
	}
	var status serveStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return serveStatus{}, false
	}
	return status, true
}

// tailscaleServesPort reports whether tailscaled itself listens on port, which
// is what makes a wildcard bind to the same port impossible.
func tailscaleServesPort(port string) bool {
	status, ok := readServeStatus()
	if !ok {
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

// ServeFrontEnd describes a `tailscale serve` mapping that forwards to us.
type ServeFrontEnd struct {
	// Via is the tailnet-side "host:port" clients connect to.
	Via string
	// Funnel is set when that mapping is published to the public internet
	// rather than just the tailnet.
	Funnel bool
}

// TailscaleFrontEnd returns the serve mapping that forwards to listenAddr's
// port, if there is one. A front-end matters because it dials us from
// 127.0.0.1: every request it forwards looks local, whoever it actually came
// from.
func TailscaleFrontEnd(listenAddr string) (ServeFrontEnd, bool) {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		return ServeFrontEnd{}, false
	}
	status, ok := readServeStatus()
	if !ok {
		return ServeFrontEnd{}, false
	}

	for hostPort, web := range status.Web {
		for _, handler := range web.Handlers {
			if forwardTargetPort(handler.Proxy) == port {
				return ServeFrontEnd{Via: hostPort, Funnel: status.AllowFunnel[hostPort]}, true
			}
		}
	}
	for tcpPort, handler := range status.TCP {
		if forwardTargetPort(handler.TCPForward) == port {
			// A TCP forward has no host of its own; name it by the port
			// tailscaled listens on.
			via := ":" + tcpPort
			return ServeFrontEnd{Via: via, Funnel: funnelOnPort(status.AllowFunnel, tcpPort)}, true
		}
	}
	return ServeFrontEnd{}, false
}

// forwardTargetPort pulls the port out of a serve forward target, which
// tailscale accepts in several spellings: "http://127.0.0.1:17294",
// "127.0.0.1:17294", or a bare "17294".
func forwardTargetPort(target string) string {
	if target == "" {
		return ""
	}
	if u, err := url.Parse(target); err == nil && u.Host != "" {
		if p := u.Port(); p != "" {
			return p
		}
	}
	if _, p, err := net.SplitHostPort(target); err == nil {
		return p
	}
	if _, err := strconv.Atoi(target); err == nil {
		return target
	}
	return ""
}

// funnelOnPort reports whether funnel is enabled for any host on this port.
func funnelOnPort(allowFunnel map[string]bool, port string) bool {
	for hostPort, allowed := range allowFunnel {
		if !allowed {
			continue
		}
		if _, p, err := net.SplitHostPort(hostPort); err == nil && p == port {
			return true
		}
	}
	return false
}

// WarnIfFrontEndBypassesAuth complains, loudly and at startup, when a
// `tailscale serve` mapping forwards to this server while the loopback
// exemption is still in place.
//
// The two settings are individually reasonable and lethal together: the
// exemption exists so a browser on the machine itself needs no token, and serve
// forwards from 127.0.0.1, so every request it carries inherits that exemption.
// The result is an unauthenticated API for everyone on the tailnet - or, with
// Funnel, for the internet. Nothing in a request distinguishes the two cases,
// so the config has to, and this says so rather than waiting for someone to
// notice.
//
// Runs in the background: it shells out to the tailscale CLI, and a warning is
// not worth delaying startup for. component is "agent" or "controller", used to
// name the config key in the fix.
func WarnIfFrontEndBypassesAuth(component, listenAddr string, requireLocalAuth bool) {
	if requireLocalAuth {
		return
	}
	go func() {
		front, ok := TailscaleFrontEnd(listenAddr)
		if !ok {
			return
		}
		reach := "anyone on your tailnet"
		if front.Funnel {
			reach = "anyone on the public internet (Funnel is enabled for it)"
		}
		log.Printf("SECURITY: `tailscale serve` forwards %s to this server (%s), and %s.require_local_auth is off. "+
			"Requests arrive from 127.0.0.1, so the loopback exemption waves them all through the auth gate: %s can drive this server without the token. "+
			"Fix with: ottoman config set %s.require_local_auth true (then restart)",
			front.Via, listenAddr, component, reach, component)
	}()
}

// isWildcardHost reports whether host means "every interface".
func isWildcardHost(host string) bool {
	if host == "" || host == "*" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsUnspecified()
}
