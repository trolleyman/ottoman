package clipboard

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTextBodyValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  []byte
		status int
	}{{"unicode", []byte("Hello 👋\n£"), 0}, {"empty", nil, 0}, {"invalid utf8", []byte{0xff}, http.StatusBadRequest}, {"oversize", bytes.Repeat([]byte("x"), MaxTextBytes+1), http.StatusRequestEntityTooLarge}} {
		t.Run(tc.name, func(t *testing.T) {
			body, status, err := ReadTextBody(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(tc.value)))
			if status != tc.status {
				t.Fatalf("status %d", status)
			}
			if tc.status == 0 && (err != nil || !bytes.Equal(body, tc.value)) {
				t.Fatal("text changed")
			}
		})
	}
}
func TestReadBufferIsBounded(t *testing.T) {
	var b textBuffer
	if _, err := b.Write([]byte(strings.Repeat("x", MaxTextBytes))); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("x")); err == nil {
		t.Fatal("oversize output accepted")
	}
	if b.Len() != MaxTextBytes {
		t.Fatal("buffer grew past limit")
	}
}
