package media

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func artworkPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 600, 300))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func TestArtworkLocalAndHTTPThumbnail(t *testing.T) {
	data := artworkPNG(t)
	path := filepath.Join(t.TempDir(), "album art.png")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("artwork must not forward agent credentials")
		}
		w.Write(data)
	}))
	defer server.Close()
	for _, source := range []string{(&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/")}).String(), server.URL} {
		got, err := loadArtwork(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(got))
		if err != nil || img.Bounds().Dx() != 256 || img.Bounds().Dy() != 128 || len(got) > MaxArtworkBytes {
			t.Fatalf("invalid thumbnail: %v", err)
		}
	}
}
func TestArtworkRejectsUnsupportedOrOversizedData(t *testing.T) {
	for _, source := range []string{"", "javascript:alert(1)", "data:image/svg+xml,test", "file://other/etc/file.png", "file:relative", "https://user:pass@example.com/art"} {
		if validArtworkSource(source) {
			t.Fatalf("accepted invalid source %q", source)
		}
	}
	for _, data := range []string{"<svg xmlns='http://www.w3.org/2000/svg'></svg>", strings.Repeat("x", maxArtworkSourceBytes+1)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(data)) }))
		_, err := loadArtwork(context.Background(), server.URL)
		server.Close()
		if err == nil {
			t.Fatal("accepted unsupported artwork")
		}
	}
}
func TestArtworkKeyChangesAndSourceIsPrivate(t *testing.T) {
	st := State{Player: "player", ArtworkSource: "file:///private/album.png", Title: "Track"}
	before := artworkKey(st)
	st.Title = "Another track"
	if before == artworkKey(st) {
		t.Fatal("track change did not invalidate artwork")
	}
	data, _ := json.Marshal(st)
	if bytes.Contains(data, []byte("/private")) {
		t.Fatal("local artwork source leaked into metadata")
	}
	if _, err := Artwork(context.Background(), "bad"); err == nil {
		t.Fatal("accepted invalid artwork key")
	}
}
