// Package webos implements a minimal client for the LG webOS "SSAP" protocol
// (JSON over a websocket to wss://<tv>:3001), enough to power the TV off,
// control volume/mute, switch inputs, and set the OLED backlight. Power-on is
// done out of band via Wake-on-LAN (see PowerOn), since the SSAP socket is only
// reachable while the TV is awake.
package webos

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/pkg/errors"
)

// Client is a connection to a webOS TV.
type Client struct {
	host string

	mu      sync.Mutex
	conn    *websocket.Conn
	nextID  int
	pending map[string]chan message
	readErr error
}

// message is one SSAP frame.
type message struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	URI     string          `json:"uri,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// New returns a client for the given TV host (IP or hostname, no scheme/port).
func New(host string) *Client {
	return &Client{host: host, pending: make(map[string]chan message)}
}

// dialURL returns the SSAP websocket URL. Port 3001 is the TLS (self-signed)
// endpoint used by modern webOS.
func (c *Client) dialURL() string {
	return fmt.Sprintf("wss://%s:3001", c.host)
}

// Connect dials the TV and performs the SSAP registration handshake using the
// given client key. If the key is empty (or the TV rejects it) the TV shows an
// on-screen pairing prompt; once the user accepts, the TV returns a client key
// which is returned here and should be persisted for future connections.
//
// The returned key equals the input key when it was already valid.
func (c *Client) Connect(ctx context.Context, clientKey string) (string, error) {
	// webOS uses a self-signed certificate; the TV is a trusted device on the
	// LAN, so skip verification.
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 - LAN device, self-signed cert
		},
	}

	conn, _, err := websocket.Dial(ctx, c.dialURL(), &websocket.DialOptions{HTTPClient: httpClient})
	if err != nil {
		return "", errors.Wrap(err, "failed to dial TV")
	}
	// webOS frames can be large (input lists etc.).
	conn.SetReadLimit(1 << 20)

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	go c.readLoop(ctx)

	newKey, err := c.register(ctx, clientKey)
	if err != nil {
		conn.Close(websocket.StatusInternalError, "registration failed")
		return "", err
	}
	return newKey, nil
}

// Close closes the connection.
func (c *Client) Close() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		conn.Close(websocket.StatusNormalClosure, "")
	}
}

func (c *Client) readLoop(ctx context.Context) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			c.mu.Lock()
			c.readErr = err
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = make(map[string]chan message)
			c.mu.Unlock()
			return
		}
		var msg message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		c.mu.Lock()
		if ch, ok := c.pending[msg.ID]; ok {
			ch <- msg
		}
		c.mu.Unlock()
	}
}

// register performs the SSAP registration handshake.
func (c *Client) register(ctx context.Context, clientKey string) (string, error) {
	manifest := registrationPayload(clientKey)
	ch := make(chan message, 4)

	c.mu.Lock()
	c.pending["register_0"] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, "register_0")
		c.mu.Unlock()
	}()

	if err := c.write(ctx, message{Type: "register", ID: "register_0", Payload: manifest}); err != nil {
		return "", err
	}

	// The TV may first reply with a PROMPT response, then a "registered"
	// message once the user accepts on screen. Allow ample time for that.
	pairCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for {
		select {
		case <-pairCtx.Done():
			return "", errors.New("timed out waiting for TV pairing (accept the prompt on the TV)")
		case msg, ok := <-ch:
			if !ok {
				return "", errors.Errorf("connection closed during registration: %v", c.readErr)
			}
			switch msg.Type {
			case "registered":
				var p struct {
					ClientKey string `json:"client-key"`
				}
				_ = json.Unmarshal(msg.Payload, &p)
				if p.ClientKey == "" {
					p.ClientKey = clientKey
				}
				return p.ClientKey, nil
			case "error":
				return "", errors.Errorf("TV rejected registration: %s", msg.Error)
			default:
				// "response" with pairingType PROMPT — keep waiting.
			}
		}
	}
}

func (c *Client) write(ctx context.Context, msg message) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("not connected")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

// request sends an SSAP request and waits for the matching response payload.
func (c *Client) request(ctx context.Context, uri string, payload any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := "req_" + strconv.Itoa(c.nextID)
	ch := make(chan message, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}

	if err := c.write(ctx, message{Type: "request", ID: id, URI: uri, Payload: raw}); err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case <-reqCtx.Done():
		return nil, errors.Errorf("timed out waiting for response to %s", uri)
	case msg, ok := <-ch:
		if !ok {
			return nil, errors.Errorf("connection closed: %v", c.readErr)
		}
		if msg.Type == "error" {
			return nil, errors.Errorf("TV error for %s: %s", uri, msg.Error)
		}
		return msg.Payload, nil
	}
}
