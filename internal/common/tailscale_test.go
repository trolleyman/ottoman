package common

import "testing"

// tlsServeJSON is a serve config in the shape tailscaled reports: an HTTPS
// front-end on 443 proxying to the agent on loopback, a raw TCP forward on
// 17295, and Funnel enabled only on the HTTPS one.
const tlsServeJSON = `{
  "TCP": {
    "443": {"HTTPS": true},
    "17295": {"TCPForward": "127.0.0.1:17295"}
  },
  "Web": {
    "hades.tail1234.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:17294"}}}
  },
  "AllowFunnel": {"hades.tail1234.ts.net:443": true}
}`

func TestTailscaleFrontEnd(t *testing.T) {
	stubServeStatus(t, tlsServeJSON, true)

	// The listen address the serve mapping proxies to - the case that matters,
	// because those requests arrive from 127.0.0.1 and look local.
	front, ok := TailscaleFrontEnd("127.0.0.1:17294")
	if !ok {
		t.Fatal("TailscaleFrontEnd(127.0.0.1:17294) found nothing, want the HTTPS mapping")
	}
	if front.Via != "hades.tail1234.ts.net:443" {
		t.Errorf("Via = %q, want the serve host:port", front.Via)
	}
	if !front.Funnel {
		t.Error("Funnel = false, want true - that mapping is published to the internet")
	}

	// A TCP forward counts too, but this one isn't funnelled.
	front, ok = TailscaleFrontEnd(":17295")
	if !ok {
		t.Fatal("TailscaleFrontEnd(:17295) found nothing, want the TCP forward")
	}
	if front.Funnel {
		t.Error("Funnel = true for the TCP forward, want false")
	}

	// A port nothing forwards to is not fronted.
	if _, ok := TailscaleFrontEnd(":17293"); ok {
		t.Error("TailscaleFrontEnd(:17293) reported a front-end for an unmapped port")
	}
}

func TestTailscaleFrontEndWithoutTailscale(t *testing.T) {
	stubServeStatus(t, "", false)
	if _, ok := TailscaleFrontEnd("127.0.0.1:17294"); ok {
		t.Error("reported a front-end with no tailscale installed")
	}

	stubServeStatus(t, "not json", true)
	if _, ok := TailscaleFrontEnd("127.0.0.1:17294"); ok {
		t.Error("reported a front-end from unparseable output")
	}
}

func TestForwardTargetPort(t *testing.T) {
	// tailscale accepts several spellings of the same forward target, and
	// missing one would mean silently failing to warn.
	cases := map[string]string{
		"http://127.0.0.1:17294":  "17294",
		"https://localhost:17294": "17294",
		"127.0.0.1:17294":         "17294",
		"[::1]:17294":             "17294",
		"17294":                   "17294",
		"":                        "",
		"/var/run/socket":         "",
	}
	for target, want := range cases {
		if got := forwardTargetPort(target); got != want {
			t.Errorf("forwardTargetPort(%q) = %q, want %q", target, got, want)
		}
	}
}
