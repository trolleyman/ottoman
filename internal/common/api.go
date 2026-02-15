package common

import (
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/web"
)

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteError writes an error response
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, api.ErrorResponse{
		Code:  status,
		Error: message,
	})
}

// ReadJSON reads JSON from a request body
func ReadJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// SetupSPAHandler sets up the SPA handler
func SetupSPAHandler(router *http.ServeMux) error {
	// Embedded web (SPA fallback for all other routes)
	webFS, err := web.DistFS()
	if err != nil {
		return errors.Wrap(err, "failed to create dist/ FS")
	}

	router.Handle("/", http.FileServerFS(webFS))
	return nil
}
