package agent

import (
	"context"

	"github.com/trolleyman/ottoman/internal/api"
)

// --- API handlers ---

// PairMonitor implements api.StrictServerInterface. It starts on-screen
// pairing for a TV-backed monitor.
func (a *Agent) PairMonitor(ctx context.Context, request api.PairMonitorRequestObject) (api.PairMonitorResponseObject, error) {
	if request.Body == nil || request.Body.Edid == "" {
		return api.PairMonitor400JSONResponse{Code: 400, Error: "edid is required"}, nil
	}
	if err := a.tv.StartPairing(request.Body.Edid); err != nil {
		return api.PairMonitor500JSONResponse{Code: 500, Error: err.Error()}, nil
	}
	msg := "Pairing started — accept the prompt on the TV"
	return api.PairMonitor200JSONResponse{Success: true, Message: &msg}, nil
}

// SetMonitorVolume implements api.StrictServerInterface. Volume control is
// only available on TV-backed monitors.
func (a *Agent) SetMonitorVolume(ctx context.Context, request api.SetMonitorVolumeRequestObject) (api.SetMonitorVolumeResponseObject, error) {
	if request.Body == nil || request.Body.Edid == "" {
		return api.SetMonitorVolume400JSONResponse{Code: 400, Error: "edid is required"}, nil
	}
	edid := request.Body.Edid
	if request.Body.Volume != nil {
		if err := a.tv.SetVolume(ctx, edid, *request.Body.Volume); err != nil {
			return api.SetMonitorVolume500JSONResponse{Code: 500, Error: err.Error()}, nil
		}
	}
	if request.Body.Muted != nil {
		if err := a.tv.SetMute(ctx, edid, *request.Body.Muted); err != nil {
			return api.SetMonitorVolume500JSONResponse{Code: 500, Error: err.Error()}, nil
		}
	}
	msg := "volume updated"
	return api.SetMonitorVolume200JSONResponse{Success: true, Message: &msg}, nil
}

// SetMonitorInput implements api.StrictServerInterface. It switches a
// TV-backed monitor's external input.
func (a *Agent) SetMonitorInput(ctx context.Context, request api.SetMonitorInputRequestObject) (api.SetMonitorInputResponseObject, error) {
	if request.Body == nil || request.Body.Edid == "" || request.Body.Input == "" {
		return api.SetMonitorInput400JSONResponse{Code: 400, Error: "edid and input are required"}, nil
	}
	if err := a.tv.SwitchInput(ctx, request.Body.Edid, request.Body.Input); err != nil {
		return api.SetMonitorInput500JSONResponse{Code: 500, Error: err.Error()}, nil
	}
	msg := "input switched"
	return api.SetMonitorInput200JSONResponse{Success: true, Message: &msg}, nil
}
