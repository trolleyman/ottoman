package common

import (
	"net"
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

func TestListenWithRetryFailsFastOnOtherErrors(t *testing.T) {
	start := time.Now()
	if _, err := ListenWithRetry("tcp", "256.256.256.256:1"); err == nil {
		t.Fatal("expected an error for an invalid address")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("a non-retryable error took %s; it should fail immediately", elapsed)
	}
}
