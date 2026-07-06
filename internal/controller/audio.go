package controller

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/trolleyman/ottoman/internal/api"
)

// GetAudioSinks implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) GetAudioSinks(ctx context.Context, request api.GetAudioSinksRequestObject) (api.GetAudioSinksResponseObject, error) {
	return proxyRequest(ctx, c, "GET", "/api/audio/sinks", nil, func(resp *http.Response) (api.GetAudioSinksResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.AudioSinksResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.GetAudioSinks200JSONResponse(result), nil
		case http.StatusUnauthorized:
			return api.GetAudioSinks401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.GetAudioSinks500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.GetAudioSinks502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// SetAudioVolume implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) SetAudioVolume(ctx context.Context, request api.SetAudioVolumeRequestObject) (api.SetAudioVolumeResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/audio/volume", body, func(resp *http.Response) (api.SetAudioVolumeResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.AudioResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SetAudioVolume200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SetAudioVolume400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.SetAudioVolume401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetAudioVolume500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.SetAudioVolume502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}
