package auth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoginFailureLimiterOnlyCountsFailures(t *testing.T) {
	const limit = 13
	window := 3 * time.Minute
	limiter := newLoginFailureLimiter(window, limit)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	// 没有失败记录时，连续成功校验不会消耗失败额度。
	for i := 0; i < limit*2; i++ {
		if !limiter.begin(now) {
			t.Fatalf("successful attempt %d should be allowed", i+1)
		}
		limiter.finish(loginAttemptSucceeded, now)
	}

	for i := 0; i < limit; i++ {
		if !limiter.begin(now) {
			t.Fatalf("failed attempt %d should be allowed", i+1)
		}
		limiter.finish(loginAttemptFailed, now)
	}
	if limiter.begin(now) {
		t.Fatal("attempt above the failure limit should be rejected")
	}
	if !limiter.begin(now.Add(window)) {
		t.Fatal("attempt should be allowed after the failure window expires")
	}
	limiter.finish(loginAttemptIgnored, now.Add(window))
}

func TestLoginFailureLimiterClearsFailuresAfterSuccess(t *testing.T) {
	const limit = 13
	limiter := newLoginFailureLimiter(3*time.Minute, limit)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < limit-1; i++ {
		if !limiter.begin(now) {
			t.Fatalf("failed attempt %d should be allowed", i+1)
		}
		limiter.finish(loginAttemptFailed, now)
	}
	if !limiter.begin(now) {
		t.Fatal("successful attempt should be allowed")
	}
	limiter.finish(loginAttemptSucceeded, now)

	for i := 0; i < limit; i++ {
		if !limiter.begin(now) {
			t.Fatalf("failed attempt %d should be allowed after success", i+1)
		}
		limiter.finish(loginAttemptFailed, now)
	}
	if limiter.begin(now) {
		t.Fatal("attempt above the reset failure limit should be rejected")
	}
}

func TestLoginFailureLimiterReservesConcurrentAttempts(t *testing.T) {
	const limit = 13
	limiter := newLoginFailureLimiter(3*time.Minute, limit)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.begin(now) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := int(allowed.Load()); got != limit {
		t.Fatalf("reserved %d concurrent attempts, want %d", got, limit)
	}
	for i := 0; i < limit; i++ {
		limiter.finish(loginAttemptIgnored, now)
	}
}
