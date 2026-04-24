package service

import (
	"testing"
	"time"
)

func TestRetryPolicy_DefaultValues(t *testing.T) {
	rp := NewRetryPolicy()

	if rp.InitialDelay != 1*time.Second {
		t.Errorf("Expected initial delay 1s, got %v", rp.InitialDelay)
	}
	if rp.MaxDelay != 60*time.Second {
		t.Errorf("Expected max delay 60s, got %v", rp.MaxDelay)
	}
	if rp.Multiplier != 2.0 {
		t.Errorf("Expected multiplier 2.0, got %v", rp.Multiplier)
	}
}

func TestRetryPolicy_ShouldRetry_BasicCases(t *testing.T) {
	rp := NewRetryPolicy()

	tests := []struct {
		name       string
		retryCount int
		maxRetry   int
		expected   bool
	}{
		{"first retry", 0, 3, true},
		{"second retry", 1, 3, true},
		{"at max retry", 3, 3, false},
		{"over max retry", 5, 3, false},
		{"zero max retry", 0, 0, false},
		{"one retry allowed", 0, 1, true},
		{"one retry exhausted", 1, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rp.ShouldRetry(tt.retryCount, tt.maxRetry)
			if result != tt.expected {
				t.Errorf("ShouldRetry(%d, %d) = %v, want %v", tt.retryCount, tt.maxRetry, result, tt.expected)
			}
		})
	}
}

func TestRetryPolicy_GetDelay_ExponentialBackoff(t *testing.T) {
	rp := NewRetryPolicy()

	expected := []time.Duration{
		1 * time.Second,  // 2^0
		2 * time.Second,  // 2^1
		4 * time.Second,  // 2^2
		8 * time.Second,  // 2^3
		16 * time.Second, // 2^4
		32 * time.Second, // 2^5
	}

	for i, want := range expected {
		got := rp.GetDelay(i)
		if got != want {
			t.Errorf("GetDelay(%d) = %v, want %v", i, got, want)
		}
	}
}

func TestRetryPolicy_GetDelay_CappedAtMax(t *testing.T) {
	rp := NewRetryPolicy()

	// 2^6 = 64s > 60s max
	got := rp.GetDelay(6)
	if got != 60*time.Second {
		t.Errorf("GetDelay(6) = %v, want 60s (max)", got)
	}

	got = rp.GetDelay(100)
	if got != 60*time.Second {
		t.Errorf("GetDelay(100) = %v, want 60s (max)", got)
	}
}

func TestRetryPolicy_CustomPolicy(t *testing.T) {
	rp := &RetryPolicy{
		InitialDelay: 2 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   3.0,
	}

	if rp.GetDelay(0) != 2*time.Second {
		t.Errorf("Expected 2s initial delay, got %v", rp.GetDelay(0))
	}
	if rp.GetDelay(1) != 6*time.Second {
		t.Errorf("Expected 6s (2*3), got %v", rp.GetDelay(1))
	}
	if rp.GetDelay(2) != 18*time.Second {
		t.Errorf("Expected 18s (6*3), got %v", rp.GetDelay(2))
	}
	if rp.GetDelay(3) != 30*time.Second {
		t.Errorf("Expected 30s (capped at max), got %v", rp.GetDelay(3))
	}
}
