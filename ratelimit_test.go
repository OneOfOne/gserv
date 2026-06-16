package gserv

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	before := time.Now().Unix()
	l := NewLimiter(2, 20, 200)
	after := time.Now().Unix()

	if l.maxPerSecond != 2 || l.maxPerMinute != 20 || l.maxPerHour != 200 {
		t.Fatalf("unexpected limits: sec=%d min=%d hr=%d", l.maxPerSecond, l.maxPerMinute, l.maxPerHour)
	}

	if l.lastSec < before || l.lastSec > after {
		t.Fatalf("unexpected lastSec: %d, expected in [%d,%d]", l.lastSec, before, after)
	}

	if l.lastMin < before || l.lastMin > after {
		t.Fatalf("unexpected lastMin: %d, expected in [%d,%d]", l.lastMin, before, after)
	}

	if l.lastHour < before || l.lastHour > after {
		t.Fatalf("unexpected lastHour: %d, expected in [%d,%d]", l.lastHour, before, after)
	}
}

func TestLimiterAllowed_PerSecondLimit(t *testing.T) {
	l := NewLimiter(1, 100, 100)

	if d, err := l.Allowed(); err != nil {
		t.Fatalf("first request should be allowed, got d=%v err=%v", d, err)
	}

	d, err := l.Allowed()
	if err == nil {
		t.Fatal("second request should be blocked by per-second limit")
	}

	if !strings.Contains(err.Error(), "per second") {
		t.Fatalf("unexpected error: %v", err)
	}

	if d <= 0 {
		t.Fatalf("expected positive retry duration, got %v", d)
	}
}

func TestLimiterAllowed_PerMinuteLimit(t *testing.T) {
	l := NewLimiter(100, 1, 100)

	if _, err := l.Allowed(); err != nil {
		t.Fatalf("first request should be allowed: %v", err)
	}

	if _, err := l.Allowed(); err == nil {
		t.Fatal("second request should be blocked by per-minute limit")
	} else if !strings.Contains(err.Error(), "per minute") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLimiterAllowed_PerHourLimit(t *testing.T) {
	l := NewLimiter(100, 100, 1)

	if _, err := l.Allowed(); err != nil {
		t.Fatalf("first request should be allowed: %v", err)
	}

	if _, err := l.Allowed(); err == nil {
		t.Fatal("second request should be blocked by per-hour limit")
	} else if !strings.Contains(err.Error(), "per hour") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLimiterAllowed_ResetsWindows(t *testing.T) {
	l := NewLimiter(1, 1, 1)

	l.mux.Lock()
	l.reqPerSecond = l.maxPerSecond
	l.reqPerMinute = l.maxPerMinute
	l.reqPerHour = l.maxPerHour
	l.lastSec = time.Now().Add(-2 * time.Second).Unix()
	l.lastMin = time.Now().Add(-2 * time.Minute).Unix()
	l.lastHour = time.Now().Add(-2 * time.Hour).Unix()
	l.mux.Unlock()

	if _, err := l.Allowed(); err != nil {
		t.Fatalf("request should be allowed after windows reset: %v", err)
	}
}

func TestLimiterRequestsLeftAndLastAction(t *testing.T) {
	l := NewLimiter(3, 4, 5)

	l.mux.Lock()
	l.reqPerSecond = 10
	l.reqPerMinute = 3
	l.reqPerHour = 1
	l.lastSec = 100
	l.lastMin = 200
	l.lastHour = 150
	l.mux.Unlock()

	sec, min, hr := l.RequestsLeft()
	if sec != 0 || min != 1 || hr != 4 {
		t.Fatalf("unexpected requests left: sec=%d min=%d hr=%d", sec, min, hr)
	}

	if got := l.LastAction().Unix(); got != 200 {
		t.Fatalf("unexpected last action: %d", got)
	}
}

func TestLimiterAllowed_ConcurrentAccounting(t *testing.T) {
	const n = 100
	l := NewLimiter(n, n, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = l.Allowed()
		}()
	}
	wg.Wait()

	l.mux.RLock()
	allowed := l.totalAllowed
	blocked := l.totalBlocked
	l.mux.RUnlock()

	if allowed+blocked != n {
		t.Fatalf("unexpected accounting totals: allowed=%d blocked=%d total=%d expected=%d", allowed, blocked, allowed+blocked, n)
	}
}
