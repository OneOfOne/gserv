package sse

import (
	"runtime"
	"sync"

	"go.oneofone.dev/gserv"
	"go.oneofone.dev/oerrs"
)

const (
	ErrNoListener = oerrs.String("no registered listener")
)

type dataChan chan []byte

type multiStream struct {
	clients map[dataChan]struct{}
	mux     sync.Mutex
	data    chan []byte
}

func (ms *multiStream) add(ch dataChan) {
	ms.mux.Lock()
	ms.clients[ch] = struct{}{}
	ms.mux.Unlock()
}

func (ms *multiStream) remove(ch dataChan) (isEmpty bool) {
	ms.mux.Lock()
	delete(ms.clients, ch)
	isEmpty = len(ms.clients) == 0
	close(ch)
	ms.mux.Unlock()

	return isEmpty
}

func (ms *multiStream) close() {
	close(ms.data)
}

func (ms *multiStream) process() {
	for b := range ms.data {
		if b == nil {
			return
		}

		ms.mux.Lock()
		for ch := range ms.clients {
			trySend(ch, b)
		}
		ms.mux.Unlock()
	}
}

func NewRouter() *Router {
	return &Router{
		mss: make(map[string]*multiStream, runtime.NumCPU()),
	}
}

type Router struct {
	mux sync.RWMutex
	mss map[string]*multiStream
}

func (r *Router) getOrMake(id string) (ms *multiStream) {
	r.mux.Lock()
	if ms = r.mss[id]; ms == nil {
		ms = &multiStream{
			clients: make(map[dataChan]struct{}, 8),
			data:    make(chan []byte),
		}
		go ms.process()
		r.mss[id] = ms
	}
	r.mux.Unlock()

	return ms
}

func (r *Router) removeIfEmpty(ms *multiStream, ch dataChan, id string) {
	if !ms.remove(ch) {
		return
	}

	r.mux.Lock()
	if ms := r.mss[id]; ms != nil {
		ms.close()
		delete(r.mss, id)
	}
	r.mux.Unlock()
}

// Handle will take over the current connection and process events
func (r *Router) Handle(id string, bufSize int, ctx *gserv.Context) (_ gserv.Response) {
	h := ctx.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	ctx.Flush()

	var (
		ch     = make(dataChan, bufSize)
		doneCh = ctx.Req.Context().Done()
		ms     = r.getOrMake(id)
	)

	ms.add(ch)

	defer r.removeIfEmpty(ms, ch, id)

	for {
		select {
		case data := <-ch:
			if _, err := ctx.Write(data); err != nil {
				return nil
			}
			ctx.Flush()
		case <-doneCh:
			return
		}
	}
}

func (r *Router) Send(id, eventID, event string, data any) (err error) {
	r.mux.RLock()
	defer r.mux.RUnlock()

	ms := r.mss[id]

	if ms == nil {
		return ErrNoListener
	}

	var b []byte
	if b, err = makeData(eventID, event, data); err != nil {
		return err
	}
	ms.data <- b

	return err
}

func trySend(ch dataChan, evt []byte) bool {
	select {
	case ch <- evt:
		return true
	default:
		return false
	}
}

func LastEventID(ctx *gserv.Context) string {
	if id := ctx.ReqHeader("Last-Event-ID"); id != "" {
		return id
	}

	return ctx.Query("lastEventID")
}
