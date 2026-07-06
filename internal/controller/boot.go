package controller

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/trolleyman/ottoman/internal/api"
)

// Boot implements api.StrictServerInterface by proxying to the agent (used to
// switch a running Linux machine into Windows without a wake cycle).
func (c *Controller) Boot(ctx context.Context, request api.BootRequestObject) (api.BootResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/boot", body, func(resp *http.Response) (api.BootResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.BootResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.Boot200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.Boot400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.Boot401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.Boot500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.Boot502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// orchestrateWindowsBoot waits for the (freshly woken) Linux agent to come
// online, then asks it to grub-reboot into Windows. GRUB runs before any
// network stack, so a wake always lands in the Linux default first; this is the
// one extra boot cycle needed to reach Windows remotely.
func (c *Controller) orchestrateWindowsBoot() {
	const (
		pollInterval = 3 * time.Second
		maxWait      = 3 * time.Minute
	)
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		if !c.agentHealthy() {
			continue
		}
		log.Printf("Agent online after wake — requesting reboot into Windows")
		if err := c.requestAgentBoot("windows"); err != nil {
			log.Printf("Failed to request Windows boot: %v", err)
		}
		return
	}
	log.Printf("Gave up waiting for agent to come online for Windows boot")
}

// agentHealthy reports whether the agent's /health endpoint responds OK.
func (c *Controller) agentHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+c.getAgentAddr()+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// requestAgentBoot posts a boot-target request to the agent.
func (c *Controller) requestAgentBoot(target string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := proxyRequest(ctx, c, "POST", "/api/boot", mustJSON(map[string]string{"target": target}),
		func(resp *http.Response) (struct{}, error) { return struct{}{}, nil })
	return err
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
