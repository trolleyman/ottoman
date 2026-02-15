//go:build !windows

package agent

import "errors"

func makeLink(src, dst string) error {
	return errors.New("shortcuts not supported on this platform")
}
