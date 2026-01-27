package common

import (
	"encoding/json"
	"net/http"
)

// API request/response types

// StatusResponse is returned by health check endpoints
type StatusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
	Uptime  string `json:"uptime,omitempty"`
}

// ErrorResponse is returned on API errors
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// WakeRequest is sent to wake a device
type WakeRequest struct {
	Target string `json:"target"` // Name or MAC address
}

// WakeResponse is returned after a wake attempt
type WakeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// SwitchLayoutRequest is sent to switch display layout
type SwitchLayoutRequest struct {
	Layout string `json:"layout"` // Layout name
}

// SwitchLayoutResponse is returned after a layout switch
type SwitchLayoutResponse struct {
	Success       bool   `json:"success"`
	CurrentLayout string `json:"current_layout,omitempty"`
	Message       string `json:"message,omitempty"`
}

// ListLayoutsResponse returns available layouts
type ListLayoutsResponse struct {
	Layouts       []string `json:"layouts"`
	CurrentLayout string   `json:"current_layout,omitempty"`
}

// PingRequest is sent to the external API for IP registration
type PingRequest struct {
	DeviceID   string `json:"device_id"`
	ExternalIP string `json:"external_ip"`
	LocalIP    string `json:"local_ip,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

// AuthRequest is used for login
type AuthRequest struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

// AuthResponse is returned after authentication
type AuthResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	Message string `json:"message,omitempty"`
}

// JSON helper functions

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteError writes an error response
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}

// ReadJSON reads JSON from a request body
func ReadJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
