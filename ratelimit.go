package gserv

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.oneofone.dev/genh"
)

type LimitKeyFn = func(ctx *Context) string

func RateLimiter(ctx context.Context, limitKey LimitKeyFn, maxPerSecond, maxPerMinute, maxPerHour int, setHeaders bool) Handler {
	ls := NewLimiters(ctx, maxPerSecond, maxPerMinute, maxPerHour)
	limitsHeader := fmt.Sprintf(`%ds, %dm, %dh`, maxPerSecond, maxPerMinute, maxPerHour)

	if limitKey == nil {
		limitKey = func(ctx *Context) string {
			return ctx.ClientIP()
		}
	}
	return func(ctx *Context) Response {
		var (
			key = limitKey(ctx)

			l      = ls.Get(key)
			h      = ctx.Header()
			d, err = l.Allowed()

			sec, min, hr = l.RequestsLeft()
		)

		if setHeaders {
			h.Set("X-Rate-Limit-Limit", limitsHeader)
			h.Set("X-Rate-Limit-Remaining", fmt.Sprintf("%ds, %dm, %dh", sec, min, hr))
		}

		if err == nil {
			return nil
		}

		ds := strconv.Itoa(int(d.Seconds() + 1))
		if setHeaders {
			h.Set("X-Rate-Limit-Reset", ds)
			h.Set("Retry-After", ds)
		}

		return NewJSONErrorResponse(http.StatusTooManyRequests, err)
	}
}

type Limiter struct {
	mux sync.RWMutex

	// max per
	maxPerSecond int64
	maxPerMinute int64
	maxPerHour   int64

	// requests per
	reqPerSecond int64
	reqPerMinute int64
	reqPerHour   int64

	lastSec  int64
	lastMin  int64
	lastHour int64

	totalAllowed int64
	totalBlocked int64
}

func NewLimiter(maxPerSecond, maxPerMinute, maxPerHour int) *Limiter {
	ts := time.Now().Unix()
	return &Limiter{
		maxPerSecond: int64(maxPerSecond),
		maxPerMinute: int64(maxPerMinute),
		maxPerHour:   int64(maxPerHour),

		lastSec:  ts,
		lastMin:  ts,
		lastHour: ts,
	}
}

// Allowed returns the duration until the next action is allowed and an error if it's longer than 0
func (l *Limiter) Allowed() (d time.Duration, err error) {
	now := time.Now().Unix()

	l.mux.Lock()
	defer func() {
		if err == nil {
			l.totalAllowed++
		} else {
			l.totalBlocked++
		}
		l.mux.Unlock()
	}()

	if now-l.lastHour > 3599 {
		l.reqPerHour = 0
		l.lastHour = now
	}

	if l.reqPerHour++; l.reqPerHour > l.maxPerHour {
		d = time.Hour - (time.Second * time.Duration(now-l.lastHour))
		return d, fmt.Errorf("%d exceeds %d/req per hour, wait %v", l.reqPerHour, l.maxPerHour, d.String())
	}

	if now-l.lastMin > 59 {
		l.reqPerMinute = 0
		l.lastMin = now
	}

	if l.reqPerMinute++; l.reqPerMinute > l.maxPerMinute {
		d = time.Minute - (time.Second * time.Duration(now-l.lastMin))
		return d, fmt.Errorf("%d exceeds %d/req per minute, wait %v", l.reqPerMinute, l.maxPerMinute, d.String())
	}

	if now-l.lastSec > 0 {
		l.lastSec = now
		l.reqPerSecond = 0
	}

	if l.reqPerSecond++; l.reqPerSecond > l.maxPerSecond {
		d = time.Second - (time.Second * time.Duration(now-l.lastSec))
		return d, fmt.Errorf("%d exceeds %d/req per second, wait %v", l.reqPerSecond, l.maxPerSecond, d.String())
	}

	return 0, nil
}

func (l *Limiter) LastAction() (t time.Time) {
	l.mux.RLock()
	t = time.Unix(max(l.lastSec, l.lastMin, l.lastHour), 0)
	l.mux.RUnlock()
	return
}

func max(vs ...int64) int64 {
	m := vs[0]
	for _, v := range vs[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func (l *Limiter) RequestsLeft() (perSecond, perMinute, perHour int64) {
	l.mux.RLock()
	perHour, perMinute, perSecond = max(0, l.maxPerHour-l.reqPerHour), max(0, l.maxPerMinute-l.reqPerMinute), max(0, l.maxPerSecond-l.reqPerSecond)
	l.mux.RUnlock()
	return
}

func NewLimiters(ctx context.Context, maxPerSecond, maxPerMinute, maxPerHour int) *Limiters {
	ls := &Limiters{
		maxPerSecond: maxPerSecond,
		maxPerMinute: maxPerMinute,
		maxPerHour:   maxPerHour,
	}
	go ls.clean()
	return ls
}

type Limiters struct {
	ctx context.Context
	m   genh.LMap[string, *Limiter]

	maxPerSecond int
	maxPerMinute int
	maxPerHour   int

	mux sync.RWMutex
}

func (ls *Limiters) clean() {
	const checkDuration = time.Hour + (time.Minute * 30)
	tk := time.NewTicker(time.Minute * 25)
	for {
		select {
		case <-ls.ctx.Done():
			return
		case t := <-tk.C:
			for _, key := range ls.m.Keys() {
				l := ls.m.Get(key)
				if t.Sub(l.LastAction()) > checkDuration {
					ls.m.Delete(key)
				}
			}

		}
	}
}

func (ls *Limiters) Get(key string) *Limiter {
	return ls.m.MustGet(key, func() *Limiter {
		return NewLimiter(ls.maxPerSecond, ls.maxPerMinute, ls.maxPerHour)
	})
}
