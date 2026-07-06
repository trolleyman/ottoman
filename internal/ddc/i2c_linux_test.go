//go:build linux

package ddc

import (
	"bytes"
	"testing"
)

func TestBusSlowerBacksOffToCeiling(t *testing.T) {
	b := &i2cBus{spacing: ddcSpacingInit}
	// Each failure doubles the spacing, capped at the ceiling.
	b.slower()
	if want := 2 * ddcSpacingInit; b.spacing != want {
		t.Fatalf("after 1 failure spacing = %v, want %v", b.spacing, want)
	}
	for range 5 {
		b.slower()
	}
	if b.spacing != ddcSpacingMax {
		t.Fatalf("after repeated failures spacing = %v, want ceiling %v", b.spacing, ddcSpacingMax)
	}
}

func TestBusFasterSpeedsUpToFloor(t *testing.T) {
	b := &i2cBus{spacing: ddcSpacingInit}
	// A speedup only happens every ddcSpeedupAfter consecutive successes.
	for range ddcSpeedupAfter - 1 {
		b.faster()
	}
	if b.spacing != ddcSpacingInit {
		t.Fatalf("spacing changed before %d successes: %v", ddcSpeedupAfter, b.spacing)
	}
	b.faster() // ddcSpeedupAfter-th success triggers a step
	if want := ddcSpacingInit - ddcSpacingStep; b.spacing != want {
		t.Fatalf("after speedup spacing = %v, want %v", b.spacing, want)
	}
	// Enough successes drive it to, but not below, the floor.
	for range 100 * ddcSpeedupAfter {
		b.faster()
	}
	if b.spacing != ddcSpacingMin {
		t.Fatalf("spacing = %v, want floor %v", b.spacing, ddcSpacingMin)
	}
}

func TestBusFailureResetsSuccessStreak(t *testing.T) {
	b := &i2cBus{spacing: ddcSpacingInit}
	for range ddcSpeedupAfter - 1 {
		b.faster()
	}
	b.slower() // a failure must reset the streak...
	b.faster() // ...so this lone success shouldn't trigger a speedup
	if b.oks != 1 {
		t.Fatalf("success streak = %d after a failure reset, want 1", b.oks)
	}
}

// The expected bytes come from the VESA DDC/CI framing and match what
// `i2cset -y <bus> 0x37 ...` would send: the checksum XORs the 0x6E write
// address with every payload byte.

func TestSetVCPMsg(t *testing.T) {
	// Set brightness (0x10) to 50 (0x0032). Checksum:
	// 0x6E ^ 0x51 ^ 0x84 ^ 0x03 ^ 0x10 ^ 0x00 ^ 0x32 = 0x9A.
	want := []byte{0x51, 0x84, 0x03, 0x10, 0x00, 0x32, 0x9A}
	if got := setVCPMsg(vcpBrightnessCode, 50); !bytes.Equal(got, want) {
		t.Errorf("setVCPMsg(brightness, 50) = % x, want % x", got, want)
	}
}

func TestGetVCPMsg(t *testing.T) {
	// Get brightness (0x10). Checksum: 0x6E ^ 0x51 ^ 0x82 ^ 0x01 ^ 0x10 = 0xAC.
	want := []byte{0x51, 0x82, 0x01, 0x10, 0xAC}
	if got := getVCPMsg(vcpBrightnessCode); !bytes.Equal(got, want) {
		t.Errorf("getVCPMsg(brightness) = % x, want % x", got, want)
	}
}

func TestDDCChecksum(t *testing.T) {
	// XOR of the write address with the set-power-standby payload.
	payload := []byte{ddcHostAddr, 0x84, 0x03, vcpPowerCode, 0x00, powerStandby}
	var want byte = ddcWriteAddr
	for _, b := range payload {
		want ^= b
	}
	if got := ddcChecksum(ddcWriteAddr, payload); got != want {
		t.Errorf("ddcChecksum = 0x%02x, want 0x%02x", got, want)
	}
}
