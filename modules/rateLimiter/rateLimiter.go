package rpmlimiter

import (
	"context"
	"time"
)

// Follows limit of tasks per minute setted via `rpm`
type RateLimiter struct {
	rpm       int // count of task per minute
	closeChan chan struct{}
	tokens    chan struct{}
}

func NewRateLimiter(ctx context.Context, rpm int) *RateLimiter {
	if rpm <= 0 {
		panic("rpm should be > 0")
	}

	rl := &RateLimiter{
		rpm:       rpm,
		closeChan: make(chan struct{}),
		tokens:    make(chan struct{}, rpm),
	}

	go rl.FillTokens(ctx)
	return rl
}

// filling channel with `rpm` to future read from this channel.
// If channel is empty - wait
// Otherwise current rpm is lower than limit - continue
func (rl *RateLimiter) FillTokens(ctx context.Context) {
	t := time.NewTicker(time.Minute / time.Duration(rl.rpm))
	defer t.Stop()

	for i := 0; i < rl.rpm; i++ {
		rl.tokens <- struct{}{}
	}

	for {
		select {
		case <-t.C:
			select {
			case rl.tokens <- struct{}{}:
				continue
			case <-ctx.Done():
				return
			default:
			}
		case <-ctx.Done():
			return
		case <-rl.closeChan:
			return
		}
	}
}

// Wait to channel refilled
func (rl *RateLimiter) Wait(ctx context.Context) error {
	select {
	case <-rl.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
