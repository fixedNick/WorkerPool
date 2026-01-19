package ratelimiter

import (
	"context"
	"time"
)

// Follows limit of tasks per minute setted via `rate`
type RateLimiter struct {
	rate      int // count of task per minute
	closeChan chan struct{}
	tokens    chan struct{}
}

func NewRateLimiter(ctx context.Context, rate int) *RateLimiter {
	if rate <= 0 {
		panic("rate should be > 0")
	}

	rl := &RateLimiter{
		rate:      rate,
		closeChan: make(chan struct{}),
		tokens:    make(chan struct{}, rate),
	}

	go rl.fillTokens(ctx)
	return rl
}

// filling channel with `rate` to future read from this channel.
// If channel is empty - wait
// Otherwise current rate is lower than limit - continue
func (rl *RateLimiter) fillTokens(ctx context.Context) {
	t := time.NewTicker(time.Second / time.Duration(rl.rate))
	defer t.Stop()

	for i := 0; i < rl.rate; i++ {
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
