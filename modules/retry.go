package modules

import "time"

type RetryManager interface {
	// Getter
	MaxRetries() int
	// to decide: should task be retried based on current error and attempt
	ShouldRetry(err error, attempt int) bool
	// delay could be based on current attempt number. Prefer to make it exponental. base * 1 << n
	GetDelay(attempt int) time.Duration
}
