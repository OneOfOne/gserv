package gserv

import (
	"compress/gzip"
	"io"
	"net/http"
	"sync"
)

const (
	acceptHeader      = "Accept-Encoding"
	contentTypeHeader = "Content-Type"
	encodingHeader    = "Content-Encoding"
	lenHeader         = "Content-Length"

	gzEnc = "gzip"
)

var gzPool = sync.Pool{
	New: func() any {
		w := gzip.NewWriter(io.Discard)
		return &gzipRW{nil, w, false}
	},
}

func getGzipRW(rw http.ResponseWriter) *gzipRW {
	rw.Header().Set(encodingHeader, gzEnc)
	grw := gzPool.Get().(*gzipRW)
	grw.ResponseWriter, grw.wrote = rw, false
	grw.gz.Reset(rw)
	return grw
}

type gzipRW struct {
	http.ResponseWriter
	gz    *gzip.Writer
	wrote bool
}

func (w *gzipRW) ensureHeaders(status int) {
	if w.wrote {
		return
	}

	w.wrote = true
	h := w.Header()
	h.Del(lenHeader)
	h.Del(acceptHeader)
	h.Set(encodingHeader, gzEnc)
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipRW) WriteHeader(status int) {
	w.ensureHeaders(status)
}

func (w *gzipRW) Flush() {
	w.ensureHeaders(http.StatusOK)
	w.gz.Flush()
	w.ResponseWriter.(http.Flusher).Flush()
}

func (w *gzipRW) Write(b []byte) (int, error) {
	w.ensureHeaders(http.StatusOK)
	return w.gz.Write(b)
}

func (w *gzipRW) Reset() {
	w.gz.Close()
	gzPool.Put(w)
}
