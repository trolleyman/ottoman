//go:build linux

package ddc

import (
	"bytes"
	"testing"
)

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
