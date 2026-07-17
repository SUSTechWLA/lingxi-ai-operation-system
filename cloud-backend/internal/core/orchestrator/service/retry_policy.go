package service

import "time"

type RetryPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

func NewRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   2.0,
	}
}

func (rp *RetryPolicy) ShouldRetry(retryCount, maxRetry int) bool {
	return retryCount < maxRetry
}

func (rp *RetryPolicy) GetDelay(retryCount int) time.Duration {
	delay := rp.InitialDelay
	for i := 0; i < retryCount; i++ {
		delay = time.Duration(float64(delay) * rp.Multiplier)
		if delay > rp.MaxDelay {
			return rp.MaxDelay
		}
	}
	return delay
}
