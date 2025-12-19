package internal

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

func DefaultRetry(maxAttemps int) RetryConfig {
	if maxAttemps <= 0 {
		maxAttemps = 3
	}
	return RetryConfig{
		MaxAttempts:  maxAttemps,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

func WithRetry(cfg RetryConfig, operation func() error) error {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		if !isRetryAble(lastErr) {
			return lastErr
		}

		if attempt < cfg.MaxAttempts {
			fmt.Fprintf(os.Stderr, "⚠ Transfer failed, retrying (%d/%d) in %v...\n",
				attempt+1, cfg.MaxAttempts, delay)
			time.Sleep(delay)

			delay = time.Duration(float64(delay) * cfg.Multiplier)
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

func isRetryAble(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	errStr := err.Error()
	retryAbleErrors := []string{
		"connection reset",
		"broken pipe",
		"connection refused",
		"i/o timeout",
		"network is unreachable",
		"no route to host",
	}

	for _, re := range retryAbleErrors {
		if strings.Contains(strings.ToLower(errStr), re) {
			return true
		}
	}

	return false
}
