package common

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestIsAddrInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("setting up listener: %v", err)
	}
	defer ln.Close()

	// Binding the same address again must be recognised as retryable.
	if _, err := net.Listen("tcp", ln.Addr().String()); err == nil {
		t.Fatal("expected a bind conflict")
	} else if !isAddrInUse(err) {
		t.Errorf("isAddrInUse(%v) = false, want true", err)
	}

	// An unrelated failure must not be treated as retryable, or a genuinely
	// misconfigured address would stall for the whole retry window.
	if _, err := net.Listen("tcp", "256.256.256.256:1"); err == nil {
		t.Fatal("expected an error for an invalid address")
	} else if isAddrInUse(err) {
		t.Errorf("isAddrInUse(%v) = true for an unrelated error", err)
	}
}

func TestListenWithRetryBindsFreePort(t *testing.T) {
	ln, err := ListenWithRetry("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenWithRetry on a free port: %v", err)
	}
	ln.Close()
}

// serveJSON is the shape `tailscale serve status --json` reports for a TLS
// front-end on 17294 and a raw TCP forward on 17295.
const serveJSON = `{
  "TCP": {"17295": {"TCPForward": "127.0.0.1:17295"}},
  "Web": {"hades.tail1234.ts.net:17294": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:17294"}}}}
}`

// stubServeStatus makes the serve lookup answer with raw for one test.
func stubServeStatus(t *testing.T, raw string, ok bool) {
	t.Helper()
	prev := tailscaleServeStatus
	tailscaleServeStatus = func() ([]byte, bool) { return []byte(raw), ok }
	t.Cleanup(func() { tailscaleServeStatus = prev })
}

func TestListenConflictHint(t *testing.T) {
	stubServeStatus(t, serveJSON, true)

	// A wildcard bind on a port tailscale serve already holds is never going to
	// succeed, so the failure has to say so rather than retry.
	for _, addr := range []string{":17294", "0.0.0.0:17294", "[::]:17295"} {
		if hint := listenConflictHint(addr); hint == "" {
			t.Errorf("listenConflictHint(%q) = \"\", want a tailscale serve explanation", addr)
		}
	}

	// A port with no serve mapping, and a bind to one specific address, are both
	// ordinary conflicts: retrying is right and a confident guess would mislead.
	if hint := listenConflictHint(":17293"); hint != "" {
		t.Errorf("listenConflictHint(\":17293\") = %q, want \"\" for an unmapped port", hint)
	}
	if hint := listenConflictHint("127.0.0.1:17294"); hint != "" {
		t.Errorf("listenConflictHint(\"127.0.0.1:17294\") = %q, want \"\" for a loopback bind", hint)
	}
}

func TestListenConflictHintWithoutTailscale(t *testing.T) {
	// No tailscale on the box (or it failed to answer): nothing to say.
	stubServeStatus(t, "", false)
	if hint := listenConflictHint(":17294"); hint != "" {
		t.Errorf("listenConflictHint(\":17294\") = %q, want \"\" when tailscale is absent", hint)
	}

	// Garbage from the CLI must not be read as a conflict either.
	stubServeStatus(t, "not json", true)
	if hint := listenConflictHint(":17294"); hint != "" {
		t.Errorf("listenConflictHint(\":17294\") = %q, want \"\" for unparseable output", hint)
	}
}

func TestListenWithRetryFailsFastOnAServeConflict(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("setting up listener: %v", err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("splitting %q: %v", ln.Addr(), err)
	}
	stubServeStatus(t, `{"TCP": {"`+port+`": {}}}`, true)

	// The port is held and tailscale serve claims it, so this must not spend the
	// retry window waiting for a holder that will never leave. Binding the
	// wildcard here would also hit the loopback listener above.
	start := time.Now()
	if _, err := ListenWithRetry("tcp", ":"+port); err == nil {
		t.Fatal("expected a bind conflict")
	} else if !strings.Contains(err.Error(), "tailscale serve") {
		t.Errorf("error %q does not mention the serve mapping", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("a permanent conflict took %s; it should fail immediately", elapsed)
	}
}

func TestListenWithRetryFailsFastOnOtherErrors(t *testing.T) {
	start := time.Now()
	if _, err := ListenWithRetry("tcp", "256.256.256.256:1"); err == nil {
		t.Fatal("expected an error for an invalid address")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("a non-retryable error took %s; it should fail immediately", elapsed)
	}
}
