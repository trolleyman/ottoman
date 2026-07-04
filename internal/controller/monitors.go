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
			return api.SetMonitorBrightness400JSONResponse{Code: resp.StatusCode, Error: "Bad Request"}, nil
		case http.StatusUnauthorized:
			return api.SetMonitorBrightness401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetMonitorBrightness500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
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
			return api.SetMonitorPower400JSONResponse{Code: resp.StatusCode, Error: "Bad Request"}, nil
		case http.StatusUnauthorized:
			return api.SetMonitorPower401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetMonitorPower500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
		default:
			return api.SetMonitorPower502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
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
			return api.SetMonitorSettings400JSONResponse{Code: resp.StatusCode, Error: "Bad Request"}, nil
		case http.StatusUnauthorized:
			return api.SetMonitorSettings401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetMonitorSettings500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
		default:
			return api.SetMonitorSettings502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}
