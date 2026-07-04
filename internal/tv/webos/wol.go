package webos

import (
	"encoding/hex"
	"net"
	"strings"

	"github.com/pkg/errors"
)

// PowerOn wakes the TV via Wake-on-LAN. LG TVs support waking over their own
// Wi-Fi (WoWLAN) when "TV On With Mobile" is enabled. The magic packet is sent
// both as a subnet broadcast and, if host is non-empty, as a unicast datagram
// to the TV's IP — broadcast can be unreliable through some Wi-Fi APs, so we do
// both.
func PowerOn(mac, host string) error {
	packet, err := buildMagicPacket(mac)
	if err != nil {
		return err
	}

	var firstErr error
	send := func(addr string) {
		conn, err := net.Dial("udp", addr)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		defer conn.Close()
		if _, err := conn.Write(packet); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	send("255.255.255.255:9")
	if host != "" {
		send(net.JoinHostPort(host, "9"))
	}
	return firstErr
}

// buildMagicPacket builds a WoL magic packet for the given MAC address.
func buildMagicPacket(macAddr string) ([]byte, error) {
	clean := strings.NewReplacer(":", "", "-", "", ".", "").Replace(strings.ToLower(macAddr))
	if len(clean) != 12 {
		return nil, errors.Errorf("MAC address must be 12 hex characters, got %d", len(clean))
	}
	mac, err := hex.DecodeString(clean)
	if err != nil {
		return nil, errors.Wrap(err, "invalid hex in MAC address")
	}

	packet := make([]byte, 102)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:], mac)
	}
	return packet, nil
}
