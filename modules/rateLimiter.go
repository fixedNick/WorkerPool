package modules

import "context"

type RateLimiter interface {
	// RateLimiter has channel with length of rate. This method should refill this channel and starts as gorutine in constructor
	FillTokens(ctx context.Context)
	// Reads from limiter channel. If no tokens availabe - wait.
	Wait(ctx context.Context) error
}
