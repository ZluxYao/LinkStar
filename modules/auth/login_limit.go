package auth

import (
	"sync"
	"time"
)

const (
	loginFailureWindow = 3 * time.Minute
	loginFailureMax    = 13
)

// loginFailureLimiter 记录全局登录失败时间，并为正在校验的请求预留失败名额。
type loginFailureLimiter struct {
	mu       sync.Mutex
	failures []time.Time
	inFlight int
	window   time.Duration
	limit    int
}

type loginAttemptResult uint8

const (
	loginAttemptIgnored loginAttemptResult = iota
	loginAttemptFailed
	loginAttemptSucceeded
)

var globalLoginFailureLimiter = newLoginFailureLimiter(loginFailureWindow, loginFailureMax)

func newLoginFailureLimiter(window time.Duration, limit int) *loginFailureLimiter {
	return &loginFailureLimiter{
		window: window,
		limit:  limit,
	}
}

// begin 清理窗口外的失败记录，并为本次密码校验预留一个名额。
func (l *loginFailureLimiter) begin(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	active := l.failures[:0]
	for _, failedAt := range l.failures {
		if failedAt.After(cutoff) {
			active = append(active, failedAt)
		}
	}
	l.failures = active

	if len(l.failures)+l.inFlight >= l.limit {
		return false
	}
	l.inFlight++
	return true
}

// finish 结束预留；密码错误写入窗口，密码正确则清空已有失败记录。
func (l *loginFailureLimiter) finish(result loginAttemptResult, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.inFlight--
	switch result {
	case loginAttemptFailed:
		l.failures = append(l.failures, now)
	case loginAttemptSucceeded:
		l.failures = l.failures[:0]
	}
}
