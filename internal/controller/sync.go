package controller

import (
	"context"
	"log"
	"time"
)

// startMirrorSync refreshes one offline mirror immediately and periodically.
// Each mirror owns its goroutine so an unavailable TV store cannot prevent
// layouts from syncing. Log only state transitions to avoid noise while offline.
func startMirrorSync(ctx context.Context, name string, interval time.Duration, sync func(context.Context) error) {
	go func() {
		lastErr := true // log the first success, including after an offline startup
		refresh := func() {
			if err := sync(ctx); err != nil {
				if !lastErr {
					log.Printf("%s sync from agent failed (using last-known mirror): %v", name, err)
				}
				lastErr = true
				return
			}
			if lastErr {
				log.Printf("%s sync from agent OK", name)
			}
			lastErr = false
		}
		refresh()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}
