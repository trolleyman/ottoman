package clipboard

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"unicode/utf8"
)

const MaxTextBytes = 1 << 20

var errTextTooLarge = errors.New("clipboard text must be smaller than 1 MB")

func ReadTextBody(w http.ResponseWriter, r *http.Request) ([]byte, int, error) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxTextBytes))
	if err != nil {
		return nil, http.StatusRequestEntityTooLarge, errTextTooLarge
	}
	if !utf8.Valid(data) {
		return nil, http.StatusBadRequest, errors.New("clipboard text must be UTF-8")
	}
	return data, 0, nil
}

type textBuffer struct{ bytes.Buffer }

func (b *textBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxTextBytes {
		return 0, errTextTooLarge
	}
	return b.Buffer.Write(p)
}
func readCommand(ctx context.Context, cmd *exec.Cmd) (string, error) {
	var output textBuffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	data := output.Bytes()
	if !utf8.Valid(data) {
		return "", errors.New("desktop clipboard does not contain UTF-8 text")
	}
	return string(data), nil
}
