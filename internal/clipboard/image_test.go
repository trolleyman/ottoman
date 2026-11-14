package clipboard

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadPNG(t *testing.T) {
	var valid bytes.Buffer
	if err := png.Encode(&valid, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	// A real PNG header advertising huge dimensions must fail before decoding.
	huge := append([]byte(nil), valid.Bytes()...)
	binary.BigEndian.PutUint32(huge[16:20], 100000)
	binary.BigEndian.PutUint32(huge[20:24], 100000)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	for _, tc := range []struct {
		name   string
		data   []byte
		status int
	}{
		{"valid", valid.Bytes(), 0},
		{"not an image", []byte("not a PNG"), http.StatusUnsupportedMediaType},
		{"oversized upload", make([]byte, MaxImageBytes+1), http.StatusRequestEntityTooLarge},
		{"huge dimensions", huge, http.StatusRequestEntityTooLarge},
		{"truncated pixels", valid.Bytes()[:33], http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/clipboard/image", bytes.NewReader(tc.data))
			out, status, err := ReadPNG(httptest.NewRecorder(), r)
			if status != tc.status {
				t.Fatalf("status %d; want %d (%v)", status, tc.status, err)
			}
			if tc.status == 0 {
				img, err := png.Decode(bytes.NewReader(out))
				if err != nil || img.Bounds().Dx() != 2 || img.Bounds().Dy() != 3 {
					t.Fatalf("normalized image invalid: %v", err)
				}
			} else if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
