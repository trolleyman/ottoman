package controller

import (
	"context"
	"testing"
)

func TestClientIsLocal(t *testing.T) {
	cases := []struct {
		name         string
		remoteAddr   string
		forwardedFor string
		publicIP     string
		want         bool
	}{
		{"direct LAN peer", "192.168.0.42:51234", "", "", true},
		{"loopback peer (tunnel agent, no XFF)", "127.0.0.1:40000", "", "", true},
		{"tunnelled from home network", "127.0.0.1:40000", "81.2.69.142", "81.2.69.142", true},
		{"tunnelled from home, XFF list", "127.0.0.1:40000", "81.2.69.142, 10.0.0.1", "81.2.69.142", true},
		{"tunnelled from elsewhere", "127.0.0.1:40000", "203.0.113.9", "81.2.69.142", false},
		{"tunnelled, public IP unknown", "127.0.0.1:40000", "81.2.69.142", "", false},
		{"IPv4-mapped LAN XFF", "127.0.0.1:40000", "::ffff:192.168.0.42", "", true},
		{"direct public peer", "203.0.113.9:1234", "", "81.2.69.142", false},
		{"garbage remote addr", "not-an-addr", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ci := clientInfo{remoteAddr: tc.remoteAddr, forwardedFor: tc.forwardedFor}
			ctx := context.WithValue(context.Background(), clientInfoKey{}, ci)
			if got := clientIsLocal(ctx, tc.publicIP); got != tc.want {
				t.Errorf("clientIsLocal(%+v, publicIP=%q) = %v, want %v", ci, tc.publicIP, got, tc.want)
			}
		})
	}

	if clientIsLocal(context.Background(), "81.2.69.142") {
		t.Error("clientIsLocal without client info in context should be false")
	}
}
