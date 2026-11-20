// Package herdr is a Go client for the Herdr socket API.
//
// Client embeds api.Client, so every request method is available as a typed
// Go method:
//
//	c, err := herdr.FromEnv()
//	edges, err := c.PaneEdges(ctx, api.PaneEdgesParams{PaneID: id})
//
// Herdr answers exactly one request per connection, so Call dials per
// request. Subscriptions hold their own long-lived connection.
package herdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/nerdwave-nick/herdr-go/api"
)

// DefaultTimeout bounds a Call whose context has no deadline.
const DefaultTimeout = 10 * time.Second

// Client talks to one Herdr server.
type Client struct {
	api.Client

	// SocketPath is the server's Unix socket.
	SocketPath string
	// Timeout bounds calls whose context has no deadline. Zero means DefaultTimeout.
	Timeout time.Duration

	seq atomic.Uint64
}

// New returns a client for the socket at path.
func New(path string) *Client {
	c := &Client{SocketPath: path}
	c.Client.Caller = c
	return c
}

// FromEnv returns a client for HERDR_SOCKET_PATH, which Herdr sets for
// plugin commands and processes running in its panes.
func FromEnv() (*Client, error) {
	p := os.Getenv("HERDR_SOCKET_PATH")
	if p == "" {
		return nil, errors.New("herdr: HERDR_SOCKET_PATH is not set (not running under Herdr?)")
	}
	return New(p), nil
}

// Error is an error response from the server.
type Error struct {
	Method  string
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e.Method == "" {
		return fmt.Sprintf("herdr: %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("herdr: %s: %s: %s", e.Method, e.Code, e.Message)
}

// HasCode reports whether err is a server error with the given code, such
// as "pane_not_found".
func HasCode(err error, code string) bool {
	var he *Error
	return errors.As(err, &he) && he.Code == code
}

type request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
}

// Call implements api.Caller.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	if params == nil {
		params = struct{}{}
	}
	if _, ok := ctx.Deadline(); !ok {
		t := c.Timeout
		if t == 0 {
			t = DefaultTimeout
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t)
		defer cancel()
	}
	conn, r, err := c.send(ctx, method, params)
	if err != nil {
		return err
	}
	defer conn.Close()
	line, err := r.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return fmt.Errorf("herdr: %s: read: %w", method, ctxErr(ctx, err))
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("herdr: %s: decode: %w", method, err)
	}
	if resp.Error != nil {
		resp.Error.Method = method
		return resp.Error
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Result, result); err != nil {
		return fmt.Errorf("herdr: %s: decode result: %w", method, err)
	}
	return nil
}

// send dials, writes one request and returns the connection for reading.
// The connection is closed when ctx is done.
func (c *Client) send(ctx context.Context, method string, params any) (net.Conn, *bufio.Reader, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("herdr: connect %s: %w", c.SocketPath, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	closer := &stopConn{Conn: conn, stop: stop}

	b, err := json.Marshal(request{ID: "go:" + strconv.FormatUint(c.seq.Add(1), 10), Method: method, Params: params})
	if err != nil {
		closer.Close()
		return nil, nil, fmt.Errorf("herdr: %s: encode: %w", method, err)
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
		closer.Close()
		return nil, nil, fmt.Errorf("herdr: %s: write: %w", method, ctxErr(ctx, err))
	}
	return closer, bufio.NewReaderSize(conn, 64<<10), nil
}

type stopConn struct {
	net.Conn
	stop func() bool
}

func (s *stopConn) Close() error {
	s.stop()
	return s.Conn.Close()
}

func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// trimLine strips the trailing newline of a protocol line.
func trimLine(b []byte) []byte { return bytes.TrimRight(b, "\r\n") }
