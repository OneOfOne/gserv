package gserv

import (
	"bytes"
	"io"
	"log"
	"os"
)

type filteredLogger struct {
	matches [][]byte
	w       io.Writer
}

func (fl *filteredLogger) Write(p []byte) (n int, err error) {
	for _, m := range fl.matches {
		if bytes.Contains(p, m) {
			return
		}
	}
	return fl.w.Write(p)
}

func FilteredLogger(flags int, msgs ...string) *log.Logger {
	var fl = &filteredLogger{w: os.Stderr}
	for _, m := range msgs {
		fl.matches = append(fl.matches, []byte(m))
	}

	return log.New(fl, "gserv: ", flags)
}
