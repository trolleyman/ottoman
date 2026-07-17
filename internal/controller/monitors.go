package controller

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/trolleyman/ottoman/internal/api"
)

// SetMonitorBrightness implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) SetMonitorBrightness(ctx context.Context, request api.SetMonitorBrightnessRequestObject) (api.SetMonitorBrightnessResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/monitors/brightness", body, func(resp *http.Response) (api.SetMonitorBrightnessResponseObject, error) {
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
}

// SetMonitorPower implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) SetMonitorPower(ctx context.Context, request api.SetMonitorPowerRequestObject) (api.SetMonitorPowerResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/monitors/power", body, func(resp *http.Response) (api.SetMonitorPowerResponseObject, error) {
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
}

// GetMonitorPowerState implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) GetMonitorPowerState(ctx context.Context, request api.GetMonitorPowerStateRequestObject) (api.GetMonitorPowerStateResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/monitors/power-state", body, func(resp *http.Response) (api.GetMonitorPowerStateResponseObject, error) {
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
}

// SetMonitorVolume implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) SetMonitorVolume(ctx context.Context, request api.SetMonitorVolumeRequestObject) (api.SetMonitorVolumeResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/monitors/volume", body, func(resp *http.Response) (api.SetMonitorVolumeResponseObject, error) {
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
