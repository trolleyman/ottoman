// Package clipboard writes validated images to the desktop clipboard.
package clipboard

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
)

const MaxImageBytes = 20 << 20
const MaxImagePixels = 4096 * 4096

// ReadPNG checks dimensions before decoding, then normalizes the image to PNG.
// Neither filenames nor a caller-supplied MIME type are trusted.
func ReadPNG(w http.ResponseWriter, r *http.Request) ([]byte, int, error) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxImageBytes))
	if err != nil {
		return nil, http.StatusRequestEntityTooLarge, fmt.Errorf("image must be smaller than 20 MB")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, http.StatusUnsupportedMediaType, fmt.Errorf("choose a PNG, JPEG, or GIF image")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxImagePixels {
		return nil, http.StatusRequestEntityTooLarge, fmt.Errorf("image exceeds the 16 megapixel limit")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("image could not be decoded")
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("image could not be converted")
	}
	return out.Bytes(), 0, nil
}
