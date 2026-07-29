package common

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "s3cret-token"

// okHandler is the thing behind the gate; a request that reaches it got through.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func request(method, path, remoteAddr string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = remoteAddr
	return r
}

func TestMiddlewareGatesRemoteAPIRequests(t *testing.T) {
	h := NewAuthenticator(testToken, false).Middleware(okHandler())

	cases := []struct {
		name string
		path string
		want int
	}{
		{"api needs credentials", "/api/monitors", http.StatusUnauthorized},
		{"layouts needs credentials", "/api/layouts", http.StatusUnauthorized},
		{"trackpad websocket needs credentials", "/api/trackpad", http.StatusUnauthorized},
		{"health stays open", "/health", http.StatusOK},
		{"login stays open", "/api/auth", http.StatusOK},
		{"auth check stays open", "/api/auth/check", http.StatusOK},
		{"spa shell stays open", "/", http.StatusOK},
		{"spa assets stay open", "/assets/index.js", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, request("GET", tc.path, "192.168.0.5:1234"))
			if w.Code != tc.want {
				t.Fatalf("%s: got %d, want %d", tc.path, w.Code, tc.want)
			}
		})
	}
}

func TestMiddlewareAcceptsEveryCredentialShape(t *testing.T) {
	a := NewAuthenticator(testToken, false)
	h := a.Middleware(okHandler())

	t.Run("bearer", func(t *testing.T) {
		r := request("GET", "/api/monitors", "192.168.0.5:1234")
		r.Header.Set("Authorization", "Bearer "+testToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", w.Code)
		}
	})

	t.Run("basic", func(t *testing.T) {
		r := request("GET", "/api/monitors", "192.168.0.5:1234")
		r.SetBasicAuth("ottoman", testToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", w.Code)
		}
	})

	t.Run("cookie", func(t *testing.T) {
		r := request("GET", "/api/monitors", "192.168.0.5:1234")
		r.AddCookie(&http.Cookie{Name: AuthCookieName, Value: a.CookieValue()})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", w.Code)
		}
	})

	t.Run("wrong token is rejected", func(t *testing.T) {
		r := request("GET", "/api/monitors", "192.168.0.5:1234")
		r.Header.Set("Authorization", "Bearer not-the-token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", w.Code)
		}
	})

	t.Run("raw token as cookie is rejected", func(t *testing.T) {
		// The cookie holds hex(sha256(token)), so presenting the token itself
		// must not be enough - otherwise the derivation buys nothing.
		r := request("GET", "/api/monitors", "192.168.0.5:1234")
		r.AddCookie(&http.Cookie{Name: AuthCookieName, Value: testToken})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", w.Code)
		}
	})
}

func TestLoopbackExemption(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:5000", "[::1]:5000"} {
		t.Run("exempt by default: "+addr, func(t *testing.T) {
			h := NewAuthenticator(testToken, false).Middleware(okHandler())
			w := httptest.NewRecorder()
			h.ServeHTTP(w, request("GET", "/api/monitors", addr))
			if w.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", w.Code)
			}
		})

		t.Run("gated when require_local_auth: "+addr, func(t *testing.T) {
			h := NewAuthenticator(testToken, true).Middleware(okHandler())
			w := httptest.NewRecorder()
			h.ServeHTTP(w, request("GET", "/api/monitors", addr))
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401", w.Code)
			}
		})
	}
}

// A forwarded header is client-controlled, so honouring it in the auth decision
// would let any remote caller claim the loopback exemption and walk straight in.
func TestForwardedHeadersCannotForgeLoopback(t *testing.T) {
	h := NewAuthenticator(testToken, false).Middleware(okHandler())
	for _, hdr := range []string{"X-Forwarded-For", "X-Real-IP"} {
		r := request("GET", "/api/monitors", "192.168.0.5:1234")
		r.Header.Set(hdr, "127.0.0.1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s spoof got %d, want 401", hdr, w.Code)
		}
	}
}

func TestEmptyTokenDisablesAuth(t *testing.T) {
	h := NewAuthenticator("", true).Middleware(okHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, request("GET", "/api/monitors", "192.168.0.5:1234"))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 when no token is configured", w.Code)
	}
}

func TestLoginSetsCookieAndCheckAgrees(t *testing.T) {
	a := NewAuthenticator(testToken, true)
	mux := http.NewServeMux()
	a.RegisterAuthRoutes(mux)

	// Wrong token: no cookie, 401.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/auth", strings.NewReader(`{"token":"wrong"}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad login got %d, want 401", w.Code)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("a failed login must not set a cookie")
	}

	// Right token: cookie issued.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/auth", strings.NewReader(`{"token":"`+testToken+`"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("good login got %d, want 200", w.Code)
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == AuthCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("login did not set the session cookie")
	}
	if !cookie.HttpOnly {
		t.Error("session cookie should be HttpOnly")
	}
	if cookie.Value == testToken {
		t.Error("session cookie must not be the raw token")
	}

	// That cookie satisfies /api/auth/check, and its absence does not.
	r := request("GET", "/api/auth/check", "192.168.0.5:1234")
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode check response: %v", err)
	}
	if !body.Authenticated {
		t.Error("check said unauthenticated despite a valid cookie")
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, request("GET", "/api/auth/check", "192.168.0.5:1234"))
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode check response: %v", err)
	}
	if body.Authenticated {
		t.Error("check said authenticated with no credential")
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	a := NewAuthenticator(testToken, true)
	mux := http.NewServeMux()
	a.RegisterAuthRoutes(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/auth/logout", nil))
	for _, c := range w.Result().Cookies() {
		if c.Name == AuthCookieName {
			if c.MaxAge >= 0 {
				t.Errorf("logout cookie MaxAge = %d, want negative", c.MaxAge)
			}
			return
		}
	}
	t.Fatal("logout did not clear the session cookie")
}

func TestHealthURL(t *testing.T) {
	cases := map[string]string{
		"hades:17294":                      "http://hades:17294/health",
		"http://hades:17294":               "http://hades:17294/health",
		"https://hades.tailnet.ts.net":     "https://hades.tailnet.ts.net/health",
		"https://hades.tailnet.ts.net:443": "https://hades.tailnet.ts.net:443/health",
		"https://hades.tailnet.ts.net/":    "https://hades.tailnet.ts.net/health",
	}
	for in, want := range cases {
		if got := HealthURL(in); got != want {
			t.Errorf("HealthURL(%q) = %q, want %q", in, got, want)
		}
	}
}
