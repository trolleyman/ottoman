package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const MaxArtworkBytes = 512 * 1024
const maxArtworkSourceBytes = 5 * 1024 * 1024

func artworkKey(st State) string {
	sum := sha256.Sum256([]byte(st.Player + "\x00" + st.ArtworkSource + "\x00" + st.Title + "\x00" + st.Artist))
	return hex.EncodeToString(sum[:])
}
func validArtworkSource(source string) bool {
	if len(source) > 8192 {
		return false
	}
	u, err := url.Parse(source)
	if err != nil || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return u.Hostname() != ""
	case "file":
		return (u.Host == "" || u.Host == "localhost") && filepath.IsAbs(u.Path) && u.RawQuery == "" && u.Fragment == ""
	}
	return false
}

// Artwork serves only the current player's advertised art, never a caller's URL
// or file path. Local player caches work through the authenticated agent too.
func Artwork(ctx context.Context, key string) ([]byte, error) {
	if len(key) != 64 {
		return nil, errors.New("artwork unavailable")
	}
	st, err := Read(ctx)
	if err != nil || !st.Available || st.Artwork == "" || artworkKey(st) != key {
		return nil, errors.New("artwork unavailable")
	}
	return loadArtwork(ctx, st.ArtworkSource)
}
func loadArtwork(ctx context.Context, source string) ([]byte, error) {
	if !validArtworkSource(source) {
		return nil, errors.New("unsupported artwork source")
	}
	u, _ := url.Parse(source)
	var reader io.ReadCloser
	if u.Scheme == "file" {
		info, err := os.Stat(u.Path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxArtworkSourceBytes {
			return nil, errors.New("artwork unavailable")
		}
		file, err := os.Open(u.Path)
		if err != nil {
			return nil, err
		}
		reader = file
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, err
		}
		client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || (req.URL.Scheme != "http" && req.URL.Scheme != "https") || req.URL.User != nil {
				return errors.New("unsupported artwork redirect")
			}
			return nil
		}}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, errors.New("artwork unavailable")
		}
		reader = resp.Body
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxArtworkSourceBytes+1))
	if err != nil || len(data) > maxArtworkSourceBytes {
		return nil, errors.New("artwork too large")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !strings.Contains("|png|jpeg|gif|", "|"+format+"|") || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16*1024*1024 {
		return nil, errors.New("unsupported artwork image")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// Re-encode a small raster thumbnail, stripping metadata and active formats.
	width, height := config.Width, config.Height
	if width > 256 || height > 256 {
		if width >= height {
			height = max(1, height*256/width)
			width = 256
		} else {
			width = max(1, width*256/height)
			height = 256
		}
	}
	thumb := image.NewNRGBA(image.Rect(0, 0, width, height))
	bounds := img.Bounds()
	for y := range height {
		for x := range width {
			thumb.Set(x, y, img.At(bounds.Min.X+x*config.Width/width, bounds.Min.Y+y*config.Height/height))
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, thumb); err != nil {
		return nil, err
	}
	if out.Len() > MaxArtworkBytes {
		return nil, errors.New("artwork too large")
	}
	return out.Bytes(), nil
}
