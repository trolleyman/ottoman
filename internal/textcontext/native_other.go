//go:build !linux && !windows

package textcontext

import "context"

func Native(context.Context) (Backend, error) { return nil, ErrUnavailable }
