package agent

import (
	"context"
	"log"

	"github.com/trolleyman/ottoman/internal/api"
)

// GetAudioSinks implements api.StrictServerInterface.
func (a *Agent) GetAudioSinks(ctx context.Context, request api.GetAudioSinksRequestObject) (api.GetAudioSinksResponseObject, error) {
	if a.audio == nil {
		return api.GetAudioSinks500JSONResponse{Code: 500, Error: "audio control is not available on this host"}, nil
	}

	sinks, err := a.audio.ListSinks()
	if err != nil {
		return api.GetAudioSinks500JSONResponse{Code: 500, Error: err.Error()}, nil
	}

	resp := make(api.GetAudioSinks200JSONResponse, 0, len(sinks))
	for _, s := range sinks {
		resp = append(resp, api.AudioSink{
			Id:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			Volume:      s.Volume,
			Muted:       s.Muted,
			Default:     s.Default,
		})
	}
	return resp, nil
}

// SetAudioVolume implements api.StrictServerInterface. The request may carry any
// combination of volume, mute, and default; each present field is applied.
func (a *Agent) SetAudioVolume(ctx context.Context, request api.SetAudioVolumeRequestObject) (api.SetAudioVolumeResponseObject, error) {
	if a.audio == nil {
		return api.SetAudioVolume500JSONResponse{Code: 500, Error: "audio control is not available on this host"}, nil
	}
	if request.Body == nil || request.Body.Name == "" {
		return api.SetAudioVolume400JSONResponse{Code: 400, Error: "sink name is required"}, nil
	}
	name := request.Body.Name

	if request.Body.Volume != nil {
		if err := a.audio.SetVolume(name, *request.Body.Volume); err != nil {
			return api.SetAudioVolume500JSONResponse{Code: 500, Error: err.Error()}, nil
		}
	}
	if request.Body.Muted != nil {
		if err := a.audio.SetMute(name, *request.Body.Muted); err != nil {
			return api.SetAudioVolume500JSONResponse{Code: 500, Error: err.Error()}, nil
		}
	}
	if request.Body.Default != nil && *request.Body.Default {
		if err := a.audio.SetDefault(name); err != nil {
			return api.SetAudioVolume500JSONResponse{Code: 500, Error: err.Error()}, nil
		}
	}

	log.Printf("Audio: updated sink %q", name)
	msg := "audio updated"
	return api.SetAudioVolume200JSONResponse{Success: true, Message: &msg}, nil
}
