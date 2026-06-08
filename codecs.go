package gserv

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.oneofone.dev/genh"
)

// Common MIME types used by the codecs.
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

// Encoder is the interface for encoding data.
type Encoder interface {
	Encode(v any) error
}

// Decoder is the interface for decoding data.
type Decoder interface {
	Decode(v any) error
}

// Codec is the interface for codecs that encode and decode data with a content type.
type Codec interface {
	ContentType() string
	Decode(r io.Reader, body any) error
	Encode(w io.Writer, v any) error
}

// PlainTextCodec encodes and decodes plain text (string or byte slice).
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

// Encode encodes plain text data to the writer.
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

// JSONCodec encodes and decodes data as JSON.
type JSONCodec struct{ Indent bool }

func (JSONCodec) ContentType() string { return MimeJSON }

func (JSONCodec) Decode(r io.Reader, out any) error {
	return json.NewDecoder(r).Decode(&out)
}

// Encode encodes data as JSON to the writer.
func (j JSONCodec) Encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	if j.Indent {
		enc.SetIndent("", "\t")
	}

	return enc.Encode(v)
}

// MsgpCodec encodes and decodes data as msgpack.
type MsgpCodec struct{}

func (MsgpCodec) ContentType() string { return MimeMsgPack }

func (MsgpCodec) Decode(r io.Reader, out any) error {
	return genh.DecodeMsgpack(r, out)
}

// Encode encodes data as msgpack to the writer.
func (c MsgpCodec) Encode(w io.Writer, v any) error {
	return genh.EncodeMsgpack(w, v)
}

// MixedCodec uses one codec for decoding and another for encoding.
type MixedCodec[Dec, Enc Codec] struct {
	dec Dec
	enc Enc
}

func (m MixedCodec[Dec, Enc]) ContentType() string { return m.enc.ContentType() }

func (m MixedCodec[Dec, Enc]) Decode(r io.Reader, out any) error {
	return m.dec.Decode(r, out)
}

// Encode encodes data using the encoding codec.
func (m MixedCodec[Dec, Enc]) Encode(w io.Writer, v any) error {
	return m.enc.Encode(w, v)
}

func getError(err error) HTTPError {
	if err, ok := err.(HTTPError); ok {
		return err
	}
	return &Error{Code: http.StatusBadRequest, Message: err.Error()}
}
