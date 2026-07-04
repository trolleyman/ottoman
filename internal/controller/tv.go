package controller

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/trolleyman/ottoman/internal/api"
)

// GetTVState implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) GetTVState(ctx context.Context, request api.GetTVStateRequestObject) (api.GetTVStateResponseObject, error) {
	return proxyRequest(ctx, c, "GET", "/api/tv/state", nil, func(resp *http.Response) (api.GetTVStateResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.TVStateResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.GetTVState200JSONResponse(result), nil
		case http.StatusUnauthorized:
			return api.GetTVState401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		default:
			return api.GetTVState502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// PairTV implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) PairTV(ctx context.Context, request api.PairTVRequestObject) (api.PairTVResponseObject, error) {
	return proxyRequest(ctx, c, "POST", "/api/tv/pair", nil, func(resp *http.Response) (api.PairTVResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.TVResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.PairTV200JSONResponse(result), nil
		case http.StatusUnauthorized:
			return api.PairTV401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.PairTV500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
		default:
			return api.PairTV502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// SetTVPower implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) SetTVPower(ctx context.Context, request api.SetTVPowerRequestObject) (api.SetTVPowerResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/tv/power", body, func(resp *http.Response) (api.SetTVPowerResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.TVResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SetTVPower200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SetTVPower400JSONResponse{Code: resp.StatusCode, Error: "Bad Request"}, nil
		case http.StatusUnauthorized:
			return api.SetTVPower401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetTVPower500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
		default:
			return api.SetTVPower502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// SetTVVolume implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) SetTVVolume(ctx context.Context, request api.SetTVVolumeRequestObject) (api.SetTVVolumeResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/tv/volume", body, func(resp *http.Response) (api.SetTVVolumeResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.TVResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SetTVVolume200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SetTVVolume400JSONResponse{Code: resp.StatusCode, Error: "Bad Request"}, nil
		case http.StatusUnauthorized:
			return api.SetTVVolume401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetTVVolume500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
		default:
			return api.SetTVVolume502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// SetTVInput implements api.StrictServerInterface by proxying to the agent.
func (c *Controller) SetTVInput(ctx context.Context, request api.SetTVInputRequestObject) (api.SetTVInputResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/tv/input", body, func(resp *http.Response) (api.SetTVInputResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.TVResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SetTVInput200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SetTVInput400JSONResponse{Code: resp.StatusCode, Error: "Bad Request"}, nil
		case http.StatusUnauthorized:
			return api.SetTVInput401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SetTVInput500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
		default:
			return api.SetTVInput502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}
