package controller

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/trolleyman/ottoman/internal/api"
)

// SetMonitorBrightness implements api.StrictServerInterface by proxying to the
// agent. For a mirrored TV it falls back to setting the OLED backlight directly
// when the agent is down (matching how the agent routes brightness for a TV).
func (c *Controller) SetMonitorBrightness(ctx context.Context, request api.SetMonitorBrightnessRequestObject) (api.SetMonitorBrightnessResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	resp, err := proxyRequest(ctx, c, "POST", "/api/monitors/brightness", body, func(resp *http.Response) (api.SetMonitorBrightnessResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.MonitorControlResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SetMonitorBrightness200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SetMonitorBrightness400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.SetMonitorBrightness401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetMonitorBrightness500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.SetMonitorBrightness502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
	if err != nil && request.Body != nil {
		if _, ok := c.localTVEntry(request.Body.Edid); ok {
			msg, lerr := c.localSetBacklight(ctx, request.Body.Edid, request.Body.Brightness)
			if lerr != nil {
				return api.SetMonitorBrightness500JSONResponse{Code: http.StatusInternalServerError, Error: lerr.Error()}, nil
			}
			return api.SetMonitorBrightness200JSONResponse{Success: true, Message: &msg}, nil
		}
	}
	return resp, err
}

// SetMonitorPower implements api.StrictServerInterface by proxying to the
// agent, falling back to driving a mirrored TV directly when the agent is down
// (the main point of the feature: turn the TV off after the desktop is off).
func (c *Controller) SetMonitorPower(ctx context.Context, request api.SetMonitorPowerRequestObject) (api.SetMonitorPowerResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	resp, err := proxyRequest(ctx, c, "POST", "/api/monitors/power", body, func(resp *http.Response) (api.SetMonitorPowerResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.MonitorControlResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SetMonitorPower200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SetMonitorPower400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.SetMonitorPower401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetMonitorPower500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.SetMonitorPower502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
	if err != nil && request.Body != nil {
		if _, ok := c.localTVEntry(request.Body.Edid); ok {
			msg, lerr := c.localSetPower(ctx, request.Body.Edid, request.Body.On)
			if lerr != nil {
				return api.SetMonitorPower500JSONResponse{Code: http.StatusInternalServerError, Error: lerr.Error()}, nil
			}
			return api.SetMonitorPower200JSONResponse{Success: true, Message: &msg}, nil
		}
	}
	return resp, err
}

// GetMonitorPowerState implements api.StrictServerInterface by proxying to the
// agent, falling back to probing a mirrored TV directly when the agent is down.
func (c *Controller) GetMonitorPowerState(ctx context.Context, request api.GetMonitorPowerStateRequestObject) (api.GetMonitorPowerStateResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	resp, err := proxyRequest(ctx, c, "POST", "/api/monitors/power-state", body, func(resp *http.Response) (api.GetMonitorPowerStateResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.MonitorPowerStateResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.GetMonitorPowerState200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.GetMonitorPowerState400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.GetMonitorPowerState401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.GetMonitorPowerState500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.GetMonitorPowerState502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
	if err != nil && request.Body != nil {
		if _, ok := c.localTVEntry(request.Body.Edid); ok {
			return api.GetMonitorPowerState200JSONResponse{
				Edid:       request.Body.Edid,
				Responding: c.tv.Reachable(ctx, request.Body.Edid),
			}, nil
		}
	}
	return resp, err
}

// SetMonitorVolume implements api.StrictServerInterface by proxying to the
// agent, falling back to driving a mirrored TV directly when the agent is down.
func (c *Controller) SetMonitorVolume(ctx context.Context, request api.SetMonitorVolumeRequestObject) (api.SetMonitorVolumeResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	resp, err := proxyRequest(ctx, c, "POST", "/api/monitors/volume", body, func(resp *http.Response) (api.SetMonitorVolumeResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.MonitorControlResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SetMonitorVolume200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SetMonitorVolume400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.SetMonitorVolume401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetMonitorVolume500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.SetMonitorVolume502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
	if err != nil && request.Body != nil {
		if _, ok := c.localTVEntry(request.Body.Edid); ok {
			msg, lerr := c.localSetVolume(ctx, request.Body.Edid, request.Body.Volume, request.Body.Muted)
			if lerr != nil {
				return api.SetMonitorVolume500JSONResponse{Code: http.StatusInternalServerError, Error: lerr.Error()}, nil
			}
			return api.SetMonitorVolume200JSONResponse{Success: true, Message: &msg}, nil
		}
	}
	return resp, err
}

// PairMonitor implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) PairMonitor(ctx context.Context, request api.PairMonitorRequestObject) (api.PairMonitorResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/monitors/pair", body, func(resp *http.Response) (api.PairMonitorResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.MonitorControlResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.PairMonitor200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.PairMonitor400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.PairMonitor401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.PairMonitor500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.PairMonitor502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// SetMonitorInput implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) SetMonitorInput(ctx context.Context, request api.SetMonitorInputRequestObject) (api.SetMonitorInputResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/monitors/input", body, func(resp *http.Response) (api.SetMonitorInputResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.MonitorControlResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SetMonitorInput200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SetMonitorInput400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.SetMonitorInput401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetMonitorInput500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.SetMonitorInput502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// SetMonitorSettings implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) SetMonitorSettings(ctx context.Context, request api.SetMonitorSettingsRequestObject) (api.SetMonitorSettingsResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/monitors/settings", body, func(resp *http.Response) (api.SetMonitorSettingsResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.MonitorControlResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SetMonitorSettings200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SetMonitorSettings400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.SetMonitorSettings401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetMonitorSettings500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.SetMonitorSettings502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}
