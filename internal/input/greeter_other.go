//go:build !linux

package input

import "errors"

func NewGreeterControllers() (MouseController, KeyboardController, error) {
	return nil, nil, errors.New("GDM input is supported only on Linux")
}
