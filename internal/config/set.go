package config

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// A setting is one key `ottoman config set` will write, with the parser that
// decides whether a value is acceptable. Parsing here rather than at load time
// means a typo is refused while the operator is still looking at it, instead of
// surfacing as a failure to start hours later on a machine they're not sitting
// at.
type setting struct {
	key   string
	help  string
	parse func(string) (any, error)
}

// settings is the whole settable surface, in the order `config set` lists it.
// Keys absent from here are refused: a config full of misspelled keys that
// silently do nothing is the failure mode this command exists to prevent.
//
// Only keys something actually reads are listed. agent.trackpad.* and
// agent.boot.linux_entry load into the config struct but are read by nobody
// (the trackpad tuning lives in the SPA's own settings, and booting "linux" is
// a plain reboot into the GRUB default), so offering them here would be the
// same lie in a different place.
var settings = []setting{
	{"agent.listen_address", "address the agent binds, host:port (use 127.0.0.1:port behind a TLS front-end)", parseListenAddress},
	{"agent.auth_token", "shared token the agent requires (see also: config rotate-token)", parseToken},
	{"agent.require_local_auth", "gate loopback callers too; required when a TLS front-end forwards in from 127.0.0.1", parseBool},
	{"agent.boot.windows_entry", "GRUB menuentry name for Windows, for a one-shot boot into Windows", parseNonEmpty},

	{"controller.listen_address", "address the controller binds, host:port", parseListenAddress},
	{"controller.auth_token", "shared token the controller requires; must match the agent's", parseToken},
	{"controller.require_local_auth", "gate loopback callers too; required when a TLS front-end forwards in from 127.0.0.1", parseBool},
	{"controller.agent.url", "agent base URL including scheme, e.g. https://hades.tail1234.ts.net", parseAgentURL},
	{"controller.agent.mac_address", "agent's MAC address, for Wake-on-LAN", parseMAC},
}

// lookupSetting finds the spec for key.
func lookupSetting(key string) (setting, bool) {
	for _, s := range settings {
		if s.key == key {
			return s, true
		}
	}
	return setting{}, false
}

// SettableKeys lists every key `config set` accepts, one "key - help" line each.
func SettableKeys() []string {
	width := 0
	for _, s := range settings {
		if len(s.key) > width {
			width = len(s.key)
		}
	}
	lines := make([]string, 0, len(settings))
	for _, s := range settings {
		lines = append(lines, fmt.Sprintf("  %-*s  %s", width, s.key, s.help))
	}
	return lines
}

// unknownKeyError names the closest matches, since a rejected key is usually a
// near miss (agent.token for agent.auth_token) rather than an invention.
func unknownKeyError(key string) error {
	var near []string
	for _, s := range settings {
		if strings.Contains(s.key, key) || strings.Contains(key, s.key) ||
			strings.HasSuffix(s.key, "."+lastSegment(key)) {
			near = append(near, s.key)
		}
	}
	sort.Strings(near)
	msg := fmt.Sprintf("unknown config key %q", key)
	if len(near) > 0 {
		return errors.Errorf("%s; did you mean %s?", msg, strings.Join(near, ", "))
	}
	return errors.Errorf("%s\n\nsettable keys:\n%s", msg, strings.Join(SettableKeys(), "\n"))
}

func lastSegment(key string) string {
	if i := strings.LastIndex(key, "."); i >= 0 {
		return key[i+1:]
	}
	return key
}

// --- value parsers -------------------------------------------------------

func parseListenAddress(v string) (any, error) {
	host, port, err := net.SplitHostPort(v)
	if err != nil {
		return nil, errors.Errorf("%q is not host:port (e.g. :17294 or 127.0.0.1:17294)", v)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, errors.Errorf("%q has no usable port; expected 1-65535", v)
	}
	if host != "" && host != "*" && net.ParseIP(strings.Trim(host, "[]")) == nil {
		// A hostname would only bind if it resolves to a local address, and gets
		// re-resolved at every restart - almost always a typo'd IP.
		return nil, errors.Errorf("%q is not an IP address; use an address of this machine, or leave it empty to bind every interface", host)
	}
	return v, nil
}

func parseToken(v string) (any, error) {
	if v == "" {
		return nil, errors.New("empty token disables authentication entirely; set a real token, or edit the file by hand if that is genuinely what you want")
	}
	if strings.ContainsAny(v, " \t\r\n") {
		return nil, errors.New("token must not contain whitespace")
	}
	if len(v) < 16 {
		return nil, errors.Errorf("token is %d characters; use at least 16 (config rotate-token generates one)", len(v))
	}
	return v, nil
}

func parseBool(v string) (any, error) {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil, errors.Errorf("%q is not a boolean; use true or false", v)
	}
	return b, nil
}

func parseNonEmpty(v string) (any, error) {
	if strings.TrimSpace(v) == "" {
		return nil, errors.New("value must not be empty")
	}
	return v, nil
}

func parseAgentURL(v string) (any, error) {
	a := AgentControllerConfig{URL: v}
	if _, err := a.BaseURL(); err != nil {
		return nil, err
	}
	return v, nil
}

func parseMAC(v string) (any, error) {
	if _, err := net.ParseMAC(v); err != nil {
		return nil, errors.Errorf("%q is not a MAC address (expected aa:bb:cc:dd:ee:ff)", v)
	}
	return v, nil
}

// --- writing -------------------------------------------------------------

// SetValue validates value against key's parser and writes it into the config
// file, leaving every other key alone. It returns the file written and the
// parsed value as stored.
//
// path selects the file: the explicit --config path when there is one, else the
// config that was loaded, else the platform default. The file is rewritten from
// its parsed contents, so comments and key order are not preserved.
func SetValue(path, key, value string) (string, any, error) {
	spec, ok := lookupSetting(key)
	if !ok {
		return "", nil, unknownKeyError(key)
	}
	parsed, err := spec.parse(value)
	if err != nil {
		return "", nil, errors.Wrapf(err, "invalid value for %s", key)
	}
	written, err := SetValues(path, [][2]string{{key, value}})
	if err != nil {
		return "", nil, err
	}
	return written, parsed, nil
}

// SetValues applies several key/value pairs in one write, validating them all
// before touching the file so a bad value part-way through can't leave the
// config half-updated.
func SetValues(path string, pairs [][2]string) (string, error) {
	type change struct {
		key    string
		parsed any
	}
	changes := make([]change, 0, len(pairs))
	for _, kv := range pairs {
		spec, ok := lookupSetting(kv[0])
		if !ok {
			return "", unknownKeyError(kv[0])
		}
		parsed, err := spec.parse(kv[1])
		if err != nil {
			return "", errors.Wrapf(err, "invalid value for %s", kv[0])
		}
		changes = append(changes, change{kv[0], parsed})
	}

	path = writePath(path)
	current, err := readSettings(path)
	if err != nil {
		return "", err
	}
	for _, c := range changes {
		setNested(current, strings.Split(c.key, "."), c.parsed)
		if c.key == "controller.agent.url" {
			deleteNested(current, []string{"controller", "agent", "ip_address"})
			deleteNested(current, []string{"controller", "agent", "port"})
		}
	}
	if err := writeSettings(path, current); err != nil {
		return "", err
	}
	return path, nil
}

// hasSection reports whether settings already configures the named component
// ("agent" or "controller"). Used to decide which auth_token keys a rotation
// should touch: writing the other component's token into a file that never had
// one would invent configuration nobody asked for.
func hasSection(settings map[string]any, section string) bool {
	sub, ok := settings[section].(map[string]any)
	return ok && len(sub) > 0
}

// writePath resolves which file a write lands in.
func writePath(path string) string {
	if path != "" {
		return path
	}
	if configPath != "" {
		return configPath
	}
	return DefaultConfigPath()
}

// readSettings parses the config file into a nested map. A missing file is an
// empty map, so `config set` can create one.
func readSettings(path string) (map[string]any, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, errors.Wrapf(err, "failed to stat %s", path)
	}
	r := viper.New()
	r.SetConfigType("toml")
	r.SetConfigFile(path)
	if err := r.ReadInConfig(); err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", path)
	}
	return r.AllSettings(), nil
}

// writeSettings rewrites path from settings, keeping the file's existing
// permissions - it holds the auth token, so a hardened mode must survive a
// `config set`. A file created here is 0600 (see configWriter).
func writeSettings(path string, values map[string]any) error {
	if err := ensureConfigDir(path); err != nil {
		return err
	}
	var mode os.FileMode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	w := configWriter()
	if err := w.MergeConfigMap(values); err != nil {
		return errors.Wrap(err, "failed to assemble config")
	}
	if err := w.WriteConfigAs(path); err != nil {
		return errors.Wrapf(err, "failed to write %s", path)
	}

	if mode != 0 {
		if err := os.Chmod(path, mode); err != nil {
			return errors.Wrapf(err, "failed to restore permissions on %s", path)
		}
	}
	return nil
}

// setNested writes value at a dotted path, creating intermediate tables.
func setNested(m map[string]any, path []string, value any) {
	for i, seg := range path {
		if i == len(path)-1 {
			m[seg] = value
			return
		}
		sub, ok := m[seg].(map[string]any)
		if !ok {
			sub = map[string]any{}
			m[seg] = sub
		}
		m = sub
	}
}

// deleteNested removes a dotted path if it is there.
func deleteNested(m map[string]any, path []string) {
	for i, seg := range path {
		if i == len(path)-1 {
			delete(m, seg)
			return
		}
		sub, ok := m[seg].(map[string]any)
		if !ok {
			return
		}
		m = sub
	}
}

// --- token rotation ------------------------------------------------------

// Rotation records what RotateToken changed.
type Rotation struct {
	Path  string   // config file written
	Token string   // the new token
	Keys  []string // auth_token keys updated
}

// RotateToken puts a fresh token in every auth_token key the config file
// already has. Both components must present the same token, and on this machine
// they can be spread over a config file, the gdm greeter's copy of it and the
// Pi's - three files that must agree, one of them invisible. Generating and
// writing them from one command is what stops them drifting apart.
//
// token may be empty to generate one.
func RotateToken(path, token string) (*Rotation, error) {
	if token == "" {
		generated, err := GenerateToken()
		if err != nil {
			return nil, err
		}
		token = generated
	} else if _, err := parseToken(token); err != nil {
		return nil, errors.Wrap(err, "invalid token")
	}

	path = writePath(path)
	current, err := readSettings(path)
	if err != nil {
		return nil, err
	}
	var pairs [][2]string
	var keys []string
	for _, section := range []string{"agent", "controller"} {
		if hasSection(current, section) {
			key := section + ".auth_token"
			pairs = append(pairs, [2]string{key, token})
			keys = append(keys, key)
		}
	}
	if len(pairs) == 0 {
		return nil, errors.Errorf("%s configures neither an agent nor a controller; run 'ottoman config init agent' or 'ottoman config init controller' first", path)
	}

	written, err := SetValues(path, pairs)
	if err != nil {
		return nil, err
	}
	return &Rotation{Path: written, Token: token, Keys: keys}, nil
}
