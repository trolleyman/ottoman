package controller

import (
	"context"
	"net/netip"
	"testing"
)

func TestClientIsLocal(t *testing.T) {
	homeV4 := []netip.Addr{netip.MustParseAddr("81.2.69.142")}
	homeDual := []netip.Addr{
		netip.MustParseAddr("81.2.69.142"),
		netip.MustParseAddr("2001:db8:1234:5678::1"),
	}

	cases := []struct {
		name         string
		remoteAddr   string
		forwardedFor string
		publicIPs    []netip.Addr
		want         bool
	}{
		{"direct LAN peer", "192.168.0.42:51234", "", nil, true},
		{"loopback peer (tunnel agent, no XFF)", "127.0.0.1:40000", "", nil, true},
		{"tunnelled from home network", "127.0.0.1:40000", "81.2.69.142", homeV4, true},
		{"tunnelled from home, XFF list", "127.0.0.1:40000", "81.2.69.142, 10.0.0.1", homeV4, true},
		{"tunnelled from home, XFF with port", "127.0.0.1:40000", "81.2.69.142:54321", homeV4, true},
		{"tunnelled from elsewhere", "127.0.0.1:40000", "203.0.113.9", homeV4, false},
		{"tunnelled, public IP unknown", "127.0.0.1:40000", "81.2.69.142", nil, false},
		{"IPv4-mapped LAN XFF", "127.0.0.1:40000", "::ffff:192.168.0.42", nil, true},
		{"IPv6 client in home /64", "127.0.0.1:40000", "2001:db8:1234:5678:abcd::2", homeDual, true},
		{"IPv6 client with port in home /64", "127.0.0.1:40000", "[2001:db8:1234:5678:abcd::2]:443", homeDual, true},
		{"IPv6 client outside home /64", "127.0.0.1:40000", "2001:db8:ffff:0001::2", homeDual, false},
		{"direct public peer", "203.0.113.9:1234", "", homeV4, false},
		{"garbage remote addr", "not-an-addr", "", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Controller{}
			ci := clientInfo{remoteAddr: tc.remoteAddr, forwardedFor: tc.forwardedFor}
			ctx := context.WithValue(context.Background(), clientInfoKey{}, ci)
			if got := c.clientIsLocal(ctx, tc.publicIPs); got != tc.want {
				t.Errorf("clientIsLocal(%+v, publicIPs=%v) = %v, want %v", ci, tc.publicIPs, got, tc.want)
			}
		})
	}

	if (&Controller{}).clientIsLocal(context.Background(), homeV4) {
		t.Error("clientIsLocal without client info in context should be false")
	}
}
