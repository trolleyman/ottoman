package agent

import (
	"context"
	"log"

	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/store"
)

// SetMonitorBrightness implements api.StrictServerInterface.
func (a *Agent) SetMonitorBrightness(ctx context.Context, request api.SetMonitorBrightnessRequestObject) (api.SetMonitorBrightnessResponseObject, error) {
	if request.Body == nil || request.Body.Edid == "" {
		return api.SetMonitorBrightness400JSONResponse{Code: 400, Error: "edid is required"}, nil
	}
	if err := a.control.setBrightness(request.Body.Edid, request.Body.Brightness); err != nil {
		return api.SetMonitorBrightness500JSONResponse{Code: 500, Error: err.Error()}, nil
	}
	log.Printf("Set brightness of %q to %d", request.Body.Edid, request.Body.Brightness)
	msg := "brightness updated"
	return api.SetMonitorBrightness200JSONResponse{Success: true, Message: &msg}, nil
}

// SetMonitorPower implements api.StrictServerInterface.
func (a *Agent) SetMonitorPower(ctx context.Context, request api.SetMonitorPowerRequestObject) (api.SetMonitorPowerResponseObject, error) {
	if request.Body == nil || request.Body.Edid == "" {
		return api.SetMonitorPower400JSONResponse{Code: 400, Error: "edid is required"}, nil
	}
	if err := a.control.setPower(request.Body.Edid, request.Body.On); err != nil {
		return api.SetMonitorPower500JSONResponse{Code: 500, Error: err.Error()}, nil
	}
	log.Printf("Set power of %q to on=%v", request.Body.Edid, request.Body.On)
	msg := "power updated"
	return api.SetMonitorPower200JSONResponse{Success: true, Message: &msg}, nil
}

// SetMonitorSettings implements api.StrictServerInterface. It updates the
// monitor's persisted registry entry (friendly name, control backend, control
// visibility).
func (a *Agent) SetMonitorSettings(ctx context.Context, request api.SetMonitorSettingsRequestObject) (api.SetMonitorSettingsResponseObject, error) {
	if request.Body == nil || request.Body.Edid == "" {
		return api.SetMonitorSettings400JSONResponse{Code: 400, Error: "edid is required"}, nil
	}
	body := request.Body

	if _, err := a.registry.Update(body.Edid, func(e *store.MonitorEntry) {
		if body.FriendlyName != nil {
			e.FriendlyName = *body.FriendlyName
		}
		if body.Backend != nil {
			e.Backend = *body.Backend
		}
		if body.Visibility != nil {
			e.Visibility = *body.Visibility
		}
		if body.Tv != nil {
			conn := &store.TVConn{Type: "webos"}
			if body.Tv.Type != nil && *body.Tv.Type != "" {
				conn.Type = *body.Tv.Type
			}
			if body.Tv.Host != nil {
				conn.Host = *body.Tv.Host
			}
			if body.Tv.Mac != nil {
				conn.Mac = *body.Tv.Mac
			}
			e.TV = conn
		}
	}); err != nil {
		return api.SetMonitorSettings500JSONResponse{Code: 500, Error: err.Error()}, nil
	}

	log.Printf("Updated registry settings for %q", body.Edid)
	msg := "settings updated"
	return api.SetMonitorSettings200JSONResponse{Success: true, Message: &msg}, nil
}
