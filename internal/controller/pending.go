package controller

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/store"
)

// Choosing a layout for a machine that is off.
//
// The desktop can't be told anything while it's asleep, so the controller holds
// the choice and applies it on the way up: pick "TV" from the phone, press wake
// (or don't - the choice survives until something answers), and the desktop
// comes up on the TV. This is also the only reliable answer to "the monitors
// are off and I want the TV": whether a powered-off monitor disappears from the
// bus is a property of the monitor, so inferring intent from what's connected
// is guesswork, while an explicit request is not.
// pendingTTL bounds how long a queued layout stays armed. Long enough to cover
// a slow boot (or a wake that needs a second try), short enough that a choice
// made and forgotten last night doesn't hijack tomorrow's boot.
const pendingTTL = 15 * time.Minute

// defaultPendingPollInterval is how often the armed orchestrator asks whether
// the agent is up. Cheap: one request, and only while something is queued. Per
// controller (c.pendingPollInterval) so a test can shorten its own without
// touching another's running orchestrator.
const defaultPendingPollInterval = 3 * time.Second

// pendingLayout is a layout chosen while the desktop was unreachable, waiting
// for it to come up.
type pendingLayout struct {
	mu sync.Mutex

	id      string    // layout id/alias, "" when nothing is queued
	expires time.Time // when the choice lapses

	// greeterDone records that the login-screen agent has already had the
	// layout applied. The GDM greeter and the user session serve the same API
	// on the same port, but a layout applied to the login screen does not carry
	// into the session - so a queued choice is applied to both, and only the
	// session's clears it.
	greeterDone bool

	// running guards the single orchestrator goroutine.
	running bool
}

// arm queues id, replacing whatever was queued before, and reports whether the
// caller should start the orchestrator.
func (p *pendingLayout) arm(id string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.id = id
	p.expires = now.Add(pendingTTL)
	p.greeterDone = false
	if p.running {
		return false
	}
	p.running = true
	return true
}

// get returns the queued layout, or "" if nothing is queued or it has lapsed.
func (p *pendingLayout) get(now time.Time) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.id == "" || now.After(p.expires) {
		return ""
	}
	return p.id
}

// clear drops the queued layout and stops the orchestrator.
func (p *pendingLayout) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.id = ""
	p.greeterDone = false
	p.running = false
}

// markGreeterApplied records that the login screen got the layout; the session
// still needs it.
func (p *pendingLayout) markGreeterApplied() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.greeterDone = true
}

// needsApply reports whether the agent that just answered should be sent the
// queued layout: the session agent always, the greeter only the first time.
func (p *pendingLayout) needsApply(greeter bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if greeter {
		return !p.greeterDone
	}
	return true
}

// PendingLayout returns the layout queued for the next time the desktop comes
// up, or "".
func (c *Controller) PendingLayout() string { return c.pending.get(time.Now()) }

// queueLayout arms a layout to be applied when the agent next answers, and
// starts the orchestrator if it isn't already running.
func (c *Controller) queueLayout(id string) {
	if c.pending.arm(id, time.Now()) {
		go c.applyPendingWhenUp()
	}
	log.Printf("Layout %q queued for when the desktop comes up (expires in %s)", id, pendingTTL)
}

// applyPendingWhenUp polls until the agent answers, then applies the queued
// layout. It keeps the choice armed across the login screen: the greeter gets
// it so the login screen lands on the right display, and the session agent gets
// it again when it takes over, because Mutter restores the session's own layout
// on login regardless of what the greeter did.
func (c *Controller) applyPendingWhenUp() {
	defer c.pending.clear()

	for {
		time.Sleep(c.pendingPollInterval)

		id := c.pending.get(time.Now())
		if id == "" {
			log.Printf("Queued layout lapsed before the desktop came up")
			return
		}

		status, ok := c.agentStatus()
		if !ok {
			continue
		}
		greeter := status.Greeter != nil && *status.Greeter
		if !c.pending.needsApply(greeter) {
			continue
		}

		if err := c.requestAgentLayout(id); err != nil {
			log.Printf("Queued layout %q failed to apply: %v", id, err)
			continue
		}
		if greeter {
			// The login screen is on the right display now; the session will
			// come up on Mutter's own config, so stay armed for it.
			log.Printf("Queued layout %q applied to the login screen; still waiting for the session", id)
			c.pending.markGreeterApplied()
			continue
		}
		log.Printf("Queued layout %q applied", id)
		return
	}
}

// agentStatus fetches the agent's status, which also says whether the responder
// is the login-screen agent. Not reachable / not answering is (zero, false).
func (c *Controller) agentStatus() (api.StatusResponse, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	status, err := proxyRequest(ctx, c, "GET", "/api/status", nil, func(resp *http.Response) (api.StatusResponse, error) {
		if resp.StatusCode != http.StatusOK {
			return api.StatusResponse{}, errors.Errorf("agent status %d", resp.StatusCode)
		}
		var s api.StatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			return api.StatusResponse{}, err
		}
		return s, nil
	})
	return status, err == nil
}

// requestAgentLayout asks the agent to switch layout.
func (c *Controller) requestAgentLayout(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return errors.Wrap(func() error {
		_, err := proxyRequest(ctx, c, "POST", "/api/layouts/switch", mustJSON(map[string]string{"layout": id}),
			func(resp *http.Response) (struct{}, error) {
				if resp.StatusCode != http.StatusOK {
					return struct{}{}, errors.New(agentErrorMessage(resp, http.StatusText(resp.StatusCode)))
				}
				return struct{}{}, nil
			})
		return err
	}(), "switch layout")
}

// --- layouts mirror -------------------------------------------------------

// layoutsMirrorFile is where the controller keeps the agent's layouts so the UI
// can still list them - and queue one - while the desktop is off. Without it
// the layouts view is empty exactly when you need it: choosing where the
// machine should come up.
const layoutsMirrorFile = "controller-layouts.json"

func layoutsMirrorDir() string  { return store.DataDir() }
func layoutsMirrorPath() string { return filepath.Join(layoutsMirrorDir(), layoutsMirrorFile) }

// syncLayoutsFromAgent mirrors the agent's layouts to disk. Best-effort: a
// failure leaves the previous mirror in place.
func (c *Controller) syncLayoutsFromAgent(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	resp, err := proxyRequest(ctx, c, "GET", "/api/layouts", nil, func(resp *http.Response) (api.LayoutsResponse, error) {
		if resp.StatusCode != http.StatusOK {
			return api.LayoutsResponse{}, errors.Errorf("agent layouts %d", resp.StatusCode)
		}
		var l api.LayoutsResponse
		if err := json.NewDecoder(resp.Body).Decode(&l); err != nil {
			return api.LayoutsResponse{}, err
		}
		return l, nil
	})
	if err != nil {
		return err
	}

	body, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	path := layoutsMirrorPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.Wrap(err, "creating data dir")
	}
	// Temp + rename so a reader never sees a half-written file.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return errors.Wrap(err, "writing layouts mirror")
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return errors.Wrap(err, "replacing layouts mirror")
	}
	return nil
}

// mirroredLayouts reads the last-known layouts. ok is false when there is no
// usable mirror yet.
func mirroredLayouts() (api.LayoutsResponse, bool) {
	data, err := os.ReadFile(layoutsMirrorPath())
	if err != nil {
		return api.LayoutsResponse{}, false
	}
	var l api.LayoutsResponse
	if err := json.Unmarshal(data, &l); err != nil {
		return api.LayoutsResponse{}, false
	}
	return l, true
}

// layoutsSyncInterval is how often the mirror refreshes while the desktop is
// up. Layouts change rarely; what matters is that the copy is current by the
// moment the desktop goes down and the list is all the UI has left.
const layoutsSyncInterval = 30 * time.Second

// startLayoutsSync keeps the layouts mirror fresh, once immediately and then on
// a ticker. Separate from the TV sync because it must run even when the TV
// mirror is unavailable.
func (c *Controller) startLayoutsSync(ctx context.Context) {
	go func() {
		lastErr := true // force a one-time "sync ok" log on first success
		sync := func() {
			if err := c.syncLayoutsFromAgent(ctx); err != nil {
				if !lastErr {
					log.Printf("Layout sync from agent failed (using last-known mirror): %v", err)
				}
				lastErr = true
				return
			}
			if lastErr {
				log.Printf("Layout sync from agent OK")
			}
			lastErr = false
		}
		sync()
		ticker := time.NewTicker(layoutsSyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sync()
			}
		}
	}()
}
