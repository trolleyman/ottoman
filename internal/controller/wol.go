package controller

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"
)

// WakeOnLAN sends a magic packet to wake a device
func WakeOnLAN(macAddr string, broadcastAddr string) error {
	// Parse MAC address
	mac, err := parseMAC(macAddr)
	if err != nil {
		return errors.Wrap(err, "invalid MAC address")
	}

	// Build magic packet
	packet := buildMagicPacket(mac)

	// Send via UDP broadcast
	if broadcastAddr == "" {
		broadcastAddr = "255.255.255.255:9"
	}

	conn, err := net.Dial("udp", broadcastAddr)
	if err != nil {
		return errors.Wrap(err, "failed to create UDP connection")
	}
	defer conn.Close()

	_, err = conn.Write(packet)
	if err != nil {
		return errors.Wrap(err, "failed to send magic packet")
	}

	return nil
}

// WakeOnLANWithPort sends a magic packet to a specific port
func WakeOnLANWithPort(macAddr string, port int) error {
	broadcastAddr := fmt.Sprintf("255.255.255.255:%d", port)
	return WakeOnLAN(macAddr, broadcastAddr)
}

// parseMAC parses a MAC address string into bytes
func parseMAC(macAddr string) ([]byte, error) {
	// Remove common separators
	macAddr = strings.ReplaceAll(macAddr, ":", "")
	macAddr = strings.ReplaceAll(macAddr, "-", "")
	macAddr = strings.ReplaceAll(macAddr, ".", "")
	macAddr = strings.ToLower(macAddr)

	if len(macAddr) != 12 {
		return nil, errors.Errorf("MAC address must be 12 hex characters, got %d", len(macAddr))
	}

	mac, err := hex.DecodeString(macAddr)
	if err != nil {
		return nil, errors.Wrap(err, "invalid hex in MAC address")
	}

	return mac, nil
}

// buildMagicPacket creates a Wake-on-LAN magic packet
// Magic packet consists of:
// - 6 bytes of 0xFF
// - Target MAC address repeated 16 times
func buildMagicPacket(mac []byte) []byte {
	packet := make([]byte, 102) // 6 + (6 * 16)

	// First 6 bytes are 0xFF
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}

	// Repeat MAC address 16 times
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:], mac)
	}

	return packet
}

// WoLTarget records one destination a magic packet was broadcast to.
type WoLTarget struct {
	Interface string `json:"interface"`
	Broadcast string `json:"broadcast"`
}

// String renders a target as "eth0→192.168.1.255".
func (t WoLTarget) String() string {
	return fmt.Sprintf("%s→%s", t.Interface, t.Broadcast)
}

// SendToAllInterfaces broadcasts a magic packet on every up, non-loopback IPv4
// interface and reports which interface/broadcast pairs it reached, so callers
// (and the UI) can show exactly where the packet went.
func SendToAllInterfaces(macAddr string) ([]WoLTarget, error) {
	mac, err := parseMAC(macAddr)
	if err != nil {
		return nil, errors.Wrap(err, "invalid MAC address")
	}

	packet := buildMagicPacket(mac)

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network interfaces")
	}

	var lastErr error
	var targets []WoLTarget

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}

			// Calculate broadcast address
			broadcast := getBroadcastAddr(ipNet)
			broadcastAddr := fmt.Sprintf("%s:9", broadcast.String())

			conn, err := net.Dial("udp", broadcastAddr)
			if err != nil {
				lastErr = err
				continue
			}

			_, err = conn.Write(packet)
			conn.Close()

			if err != nil {
				lastErr = err
				continue
			}

			targets = append(targets, WoLTarget{
				Interface: iface.Name,
				Broadcast: broadcast.String(),
			})
		}
	}

	if len(targets) == 0 {
		if lastErr != nil {
			return nil, errors.Wrap(lastErr, "failed to send on any interface")
		}
		return nil, errors.New("no usable IPv4 interface to broadcast on")
	}

	return targets, nil
}

// ValidateWakeConfig checks the configured wake target at startup and returns a
// list of human-readable warnings (empty if all is well). It verifies the MAC
// parses and that at least one up, non-loopback IPv4 interface exists to
// broadcast from.
func ValidateWakeConfig(macAddr string) []string {
	var warnings []string

	if macAddr == "" {
		return nil // no wake target configured; not a misconfiguration
	}

	if _, err := parseMAC(macAddr); err != nil {
		warnings = append(warnings, fmt.Sprintf("wake target MAC %q is invalid: %v", macAddr, err))
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("could not enumerate network interfaces: %v", err))
		return warnings
	}
	hasIPv4 := false
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				hasIPv4 = true
			}
		}
	}
	if !hasIPv4 {
		warnings = append(warnings, "no up, non-loopback IPv4 interface found - Wake-on-LAN broadcasts cannot be sent")
	}

	return warnings
}

// getBroadcastAddr calculates the broadcast address for a network
func getBroadcastAddr(ipNet *net.IPNet) net.IP {
	ip := ipNet.IP.To4()
	mask := ipNet.Mask

	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		broadcast[i] = ip[i] | ^mask[i]
	}

	return broadcast
}
