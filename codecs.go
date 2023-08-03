package gserv

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.oneofone.dev/genh"
)

// Common mime-types
const (
	MimeJSON       = "application/json"
	MimeEvent      = "text/event-stream"
	MimeMsgPack    = "application/msgpack"
	MimeXML        = "application/xml"
	MimeJavascript = "application/javascript"
	MimeHTML       = "text/html"
	MimePlain      = "text/plain"
	MimeBinary     = "application/octet-stream"
)

var (
	_ Codec = (*PlainTextCodec)(nil)
	_ Codec = (*JSONCodec)(nil)
	_ Codec = (*MsgpCodec)(nil)
	_ Codec = (*MixedCodec[JSONCodec, MsgpCodec])(nil)
)

type Encoder interface {
	Encode(v any) error
}

type Decoder interface {
	Decode(v any) error
}

type Codec interface {
	ContentType() string
	Decode(r io.Reader, body any) error
	Encode(w io.Writer, v any) error
}

type PlainTextCodec struct{}

func (PlainTextCodec) ContentType() string { return "" }

func (PlainTextCodec) Decode(r io.Reader, out any) error {
	b, err := io.ReadAll(r)
	switch out := out.(type) {
	case *string:
		*out = string(b)
	case *[]byte:
		*out = b
	case io.Writer:
		_, err = out.Write(b)
	default:
		return fmt.Errorf("%T is not a valid type for PlainTextCodec", out)
	}
	return err
}

func (PlainTextCodec) Encode(w io.Writer, v any) (err2 error) {
	switch v := v.(type) {
	case string:
		_, err2 = io.WriteString(w, v)
	case []byte:
		_, err2 = w.Write(v)
	case io.Reader:
		_, err2 = io.Copy(w, v)
	default:
		return fmt.Errorf("%T is not a valid type for PlainTextCodec", v)
	}
	return
}

type JSONCodec struct{ Indent bool }

func (JSONCodec) ContentType() string { return MimeJSON }

func (JSONCodec) Decode(r io.Reader, out any) error {
	return json.NewDecoder(r).Decode(&out)
}

func (j JSONCodec) Encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	if j.Indent {
		enc.SetIndent("", "\t")
	}

	return enc.Encode(v)
}

type MsgpCodec struct{}

func (MsgpCodec) ContentType() string { return MimeMsgPack }

func (MsgpCodec) Decode(r io.Reader, out any) error {
	return genh.DecodeMsgpack(r, out)
}

func (c MsgpCodec) Encode(w io.Writer, v any) error {
	return genh.EncodeMsgpack(w, v)
}

type MixedCodec[Dec, Enc Codec] struct {
	dec Dec
	enc Enc
}

func (m MixedCodec[Dec, Enc]) ContentType() string { return m.enc.ContentType() }

func (m MixedCodec[Dec, Enc]) Decode(r io.Reader, out any) error {
	return m.dec.Decode(r, out)
}

func (m MixedCodec[Dec, Enc]) Encode(w io.Writer, v any) error {
	return m.enc.Encode(w, v)
}

func getError(code int, err error) HTTPError {
	if err, ok := err.(HTTPError); ok {
		return err
	}
	if code == 0 {
		code = http.StatusBadRequest
	}
	return &Error{Code: code, Message: err.Error()}
}
