package common

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
)

// AuthCookieName is the cookie the web login screen sets once a client has
// presented the correct token. It holds a value derived from the token (not the
// token itself), and the browser sends it automatically on same-origin API and
// WebSocket requests - which is what covers /api/trackpad, since a browser
// cannot set an Authorization header on a WebSocket handshake.
const AuthCookieName = "ottoman_auth"

// Authenticator gates the HTTP API with the configured shared token.
//
// requireLocal decides whether loopback peers are gated too. Config defaults it
// on (see config.localAuthRequired): the exemption is only safe while nothing
// on the host forwards outside traffic in, and a request carries nothing that
// tells the two apart - a `tailscale serve` or reverse-proxy front-end dials
// from 127.0.0.1 exactly like a local browser does.
//
// An empty token disables authentication entirely.
type Authenticator struct {
	token        string // the configured shared secret ("" disables auth)
	cookie       string // hex(sha256(token)); the cookie value, so the raw token isn't stored client-side
	requireLocal bool   // gate loopback peers as well as remote ones
}

// NewAuthenticator builds an Authenticator for the given token.
func NewAuthenticator(token string, requireLocal bool) *Authenticator {
	a := &Authenticator{token: token, requireLocal: requireLocal}
	if token != "" {
		sum := sha256.Sum256([]byte(token))
		a.cookie = hex.EncodeToString(sum[:])
	}
	return a
}

// Enabled reports whether a token is configured, i.e. whether anything is gated.
func (a *Authenticator) Enabled() bool { return a != nil && a.token != "" }

// CookieValue is the value to store in AuthCookieName once a client has proved
// it knows the token.
func (a *Authenticator) CookieValue() string { return a.cookie }

// Matches reports whether a presented token is the configured one, in constant
// time. Used by the login endpoint.
func (a *Authenticator) Matches(token string) bool {
	if !a.Enabled() {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.token)) == 1
}

// openPrefixes are API paths that must stay reachable without credentials, or
// an unauthenticated browser could never reach the login screen and no client
// could ever discover that it needs to log in.
var openPrefixes = []string{
	"/health",
	"/api/auth",
}

// gated reports whether a path needs credentials. Only the API is gated: the
// SPA shell and its assets stay open so the login screen can load and do the
// exchange.
func gated(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return false
	}
	for _, p := range openPrefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return false
		}
	}
	return true
}

// isLoopback reports whether the request's immediate peer is on this machine.
//
// It deliberately reads only RemoteAddr and never a forwarded header: a header
// is client-controlled, so trusting one here would let any remote caller claim
// to be local and bypass the gate outright.
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// Authorized reports whether a request carries an acceptable credential, in any
// of the three shapes a client might use: the cookie set at login, a Bearer
// header (what the SPA and the controller-to-agent calls send), or Basic auth
// with the token as the password.
func (a *Authenticator) Authorized(r *http.Request) bool {
	if !a.Enabled() {
		return true
	}

	if c, err := r.Cookie(AuthCookieName); err == nil {
		if subtle.ConstantTimeCompare([]byte(c.Value), []byte(a.cookie)) == 1 {
			return true
		}
	}

	h := r.Header.Get("Authorization")
	if token, ok := strings.CutPrefix(h, "Bearer "); ok {
		if a.Matches(strings.TrimSpace(token)) {
			return true
		}
	}
	if _, password, ok := r.BasicAuth(); ok {
		if a.Matches(password) {
			return true
		}
	}

	return false
}

// Permits reports whether a request gets through the gate, by credential or by
// exemption. The middleware and the /api/auth/check endpoint both go through
// here, so the answer the SPA is given always matches the answer the gate will
// give - otherwise the login screen appears when it isn't needed, or worse,
// doesn't when it is.
func (a *Authenticator) Permits(r *http.Request) bool {
	if !a.Enabled() {
		return true
	}
	if !a.requireLocal && isLoopback(r.RemoteAddr) {
		return true
	}
	return a.Authorized(r)
}

// Middleware gates the API. Requests that are exempt - unprotected paths, and
// loopback peers unless requireLocal - pass straight through.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !gated(r.URL.Path) || a.Permits(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// RegisterAuthRoutes wires the session endpoints onto mux. They live here rather
// than in the generated strict handler because that handler never sees the
// ResponseWriter, so it cannot set or clear the session cookie - which is the
// whole point of the login exchange.
//
// mux must be a different ServeMux from the one carrying the generated routes,
// or these patterns collide with the generated ones and net/http panics.
func (a *Authenticator) RegisterAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/auth/check", a.handleCheckAuth)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (a *Authenticator) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "invalid request body"})
		return
	}

	// With no token configured there is nothing to prove, so say so rather than
	// leave the UI stuck on a login screen it can never satisfy.
	if !a.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	if body.Token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "missing token"})
		return
	}
	if !a.Matches(body.Token) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "invalid token"})
		return
	}

	a.SetAuthCookie(w, RequestIsSecure(r))
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *Authenticator) handleLogout(w http.ResponseWriter, r *http.Request) {
	ClearAuthCookie(w, RequestIsSecure(r))
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *Authenticator) handleCheckAuth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": a.Permits(r)})
}

// SetAuthCookie stores the derived token on the client. It is not HttpOnly-only
// by accident: the SPA never needs to read it (it sends the raw token in a
// header), so keeping scripts away from it costs nothing.
func (a *Authenticator) SetAuthCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    a.cookie,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((365 * 24 * 60 * 60)),
	})
}

// ClearAuthCookie expires the cookie on logout.
func ClearAuthCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// RequestIsSecure reports whether the request reached us over TLS, so cookies
// can carry the Secure attribute exactly when it would not lock the user out.
//
// X-Forwarded-Proto is honoured here (unlike in the auth decision) because the
// worst a lying client can do is mark its own cookie Secure and break its own
// session - it cannot use it to get past the gate.
func RequestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
