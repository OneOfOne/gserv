package rsrv

import (
	"net"
	"sync"

	"go.oneofone.dev/genh"
	"go.oneofone.dev/msgpack/v5"
)

type Conn struct {
	sync.Mutex
	net.TCPConn
	enc msgpack.Encoder
	dec msgpack.Encoder
}

func (c *Conn) Encode(v any) error {
	c.Lock()
	defer c.Unlock()
	return c.enc.Encode(v)
}

func (c *Conn) Decode(v any) error {
	c.Lock()
	defer c.Unlock()
	return c.enc.Encode(v)
}

func (c *Conn) Close() error {
	return c.TCPConn.Close()
}

func Process[In, Out any](c *Conn, onMsg func(v In) (Out, error)) error {
	for {
		if err := processOne(c, onMsg); err != nil {
			return err
		}
	}
}

func processOne[In, Out any](c *Conn, onMsg func(v In) (Out, error)) error {
	var v In
	c.Lock()
	defer c.Unlock()

	if err := genh.DecodeMsgpack(c, &v); err != nil {
		return err
	}

	resp, err := onMsg(v)
	if err != nil {
		return err
	}

	return genh.EncodeMsgpack(c, resp)
}
