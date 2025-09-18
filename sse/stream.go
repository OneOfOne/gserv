package sse

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"go.oneofone.dev/gserv"
	"go.oneofone.dev/gserv/internal"
	"go.oneofone.dev/oerrs"
)

const (
	ErrBufferFull = oerrs.String("buffer full")
)

var (
	nl        = []byte("\n")
	idBytes   = []byte("id: ")
	evtBytes  = []byte("event: ")
	dataBytes = []byte("data: ")
	pingBytes = []byte("data: ping\n\n")
)

type writeFlusher interface {
	io.Writer
	http.Flusher
}

func NewStream(ctx *gserv.Context, bufSize int) (lastEventID string, ss *Stream, err error) {
	h := ctx.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")

	ss = &Stream{
		wch:  make(chan []byte, bufSize),
		done: ctx.Req.Context().Done(),
	}
	lastEventID = LastEventID(ctx)

	go processStream(ss, ctx)

	return lastEventID, ss, err
}

type Stream struct {
	wch  chan []byte
	done <-chan struct{}
}

func (ss *Stream) send(msg []byte) error {
	select {
	case <-ss.done:
		return os.ErrClosed
	case ss.wch <- msg:
		return nil
	default:
		return ErrBufferFull
	}
}

func (ss *Stream) Ping() error {
	return ss.send(pingBytes)
}

func (ss *Stream) Retry(ms int) (err error) {
	return ss.send([]byte(fmt.Sprintf("retry: %d\n\n", ms)))
}

func (ss *Stream) SendData(data any) error {
	b, err := makeData("", "", data)
	if err != nil {
		return err
	}

	return ss.send(b)
}

func (ss *Stream) SendAll(id, evt string, msg any) error {
	b, err := makeData(id, evt, msg)
	if err != nil {
		return err
	}

	return ss.send(b)
}

func processStream(ss *Stream, ctx *gserv.Context) {
	ctx.Flush()

	for {
		select {
		case m := <-ss.wch:
			if _, err := ctx.Write(m); err != nil {
				return
			}
			ctx.Flush()
		case <-ss.done:
			return
		}
	}
}

func makeData(id, evt string, data any) ([]byte, error) {
	var buf bytes.Buffer

	if id != "" {
		buf.Write(idBytes)
		buf.WriteString(id)
		buf.WriteByte('\n')
	}

	if evt != "" {
		buf.Write(evtBytes)
		buf.WriteString(evt)
		buf.WriteByte('\n')
	}

	switch data := data.(type) {
	case nil:
		buf.WriteString("data: \n")

	case []byte:
		for p := range bytes.SplitSeq(data, nl) {
			buf.Write(dataBytes)
			buf.Write(p)
			buf.Write(nl)
		}

	case string:
		for p := range strings.SplitSeq(data, "\n") {
			buf.Write(dataBytes)
			buf.WriteString(p)
			buf.Write(nl)
		}

	default:
		v, err := internal.Marshal(data)
		if err != nil {
			return nil, err
		}

		buf.Write(dataBytes)
		buf.Write(v)
		buf.Write(nl)
	}

	buf.Write(nl)

	return buf.Bytes(), nil
}
