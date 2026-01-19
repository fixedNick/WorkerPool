package modules

import (
	"bytes"
	"errors"
	"log"
	"os"
	"time"

	"github.com/spf13/viper"
)

type RetryManager struct {
	backoff    bool          `yaml:"backoff"`
	maxRetries int           `yaml:"maxRetries"`
	delay      time.Duration `yaml:"delay"`
}

func (rm *RetryManager) MaxRetries() int {
	return rm.maxRetries
}

func NewRetryManager(cfgPath string) *RetryManager {
	_, err := os.Stat(cfgPath)
	if err != nil {
		log.Panicf("RetryManager cofig doesn't exist at [%s]. %v", cfgPath, err)
	}

	b, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Panicf("RetryManager cofig doesn't exist at [%s]. %v", cfgPath, err)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(b))
	if err != nil {
		log.Panicf("Viper readConfig error: %v", err)
	}

	rm := RetryManager{
		backoff:    viper.GetBool("delay"),
		maxRetries: viper.GetInt("maxRetries"),
		delay:      viper.GetDuration("delay") * time.Millisecond,
	}

	if err != nil {
		log.Panicf("RetryManager YAML Unmarshal error: %v", err)
	}

	return &rm
}

// Decide: should you make retry or not
// If received `err` is type of `Timeout` - we should make retry, otherwise - no
// If currentAttempt >= rm.maxRetries - no
func (rm *RetryManager) ShouldRetry(err error, attempt int) bool {
	// is current attempt valid
	if attempt >= rm.maxRetries {
		return false
	}

	// check error type
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}

// return Delay based on `rm.backoff`. If backoff is used - delay is exponental.
// otherwise - basic `rm.delay`
func (rm *RetryManager) GetDelay(attempt int) time.Duration {
	if rm.backoff {
		return rm.delay * time.Duration(1<<attempt)
	}
	return rm.delay
}
