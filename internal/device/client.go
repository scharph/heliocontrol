// Package device talks to a Heliotherm web control: it owns the connection,
// serializes request/response exchanges (the controller handles one command at
// a time) and offers typed operations on top of package protocol.
package device

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"heliocontrol/internal/frame"
	"heliocontrol/internal/protocol"
)

var (
	ErrClosed   = errors.New("device: connection closed")
	ErrTimeout  = errors.New("device: no reply")
	ErrReadOnly = errors.New("device: read-only mode, command refused")
)

const (
	// keepAliveIdle mirrors homeControl, which re-sends the login after 25 s
	// without traffic so the controller does not drop the session.
	keepAliveIdle = 25 * time.Second
	// maxCommandLen is the longest command homeControl sends (MAXLENGTH_COMMAND 245).
	maxCommandLen = 240
)

// Client is a connected session. All methods are safe for concurrent use; the
// exchanges themselves are serialized.
type Client struct {
	log      *slog.Logger
	conn     io.ReadWriteCloser
	rx       chan string
	done     chan struct{}
	err      atomic.Value // error that ended the session
	txMu     sync.Mutex
	lastTx   atomic.Int64
	once     sync.Once
	readOnly atomic.Bool
}

// SetReadOnly makes the client refuse every command that could change the
// controller (see protocol.IsReadCommand); refused commands never reach the wire.
func (c *Client) SetReadOnly(on bool) { c.readOnly.Store(on) }

func (c *Client) ReadOnly() bool { return c.readOnly.Load() }

// Dial opens the transport and starts the receive loop. It does not log in.
func Dial(ctx context.Context, cfg TransportConfig, log *slog.Logger) (*Client, error) {
	cfg.ApplyDefaults()
	conn, err := openTransport(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return newClient(conn, log), nil
}

func newClient(conn io.ReadWriteCloser, log *slog.Logger) *Client {
	if log == nil {
		log = slog.Default()
	}
	c := &Client{log: log, conn: conn, rx: make(chan string, 1024), done: make(chan struct{})}
	c.lastTx.Store(time.Now().UnixNano())
	go c.readLoop()
	go c.keepAlive()
	return c
}

// Done is closed when the connection is lost or closed.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err returns why the session ended (nil while it is alive).
func (c *Client) Err() error {
	if e, ok := c.err.Load().(error); ok {
		return e
	}
	return nil
}

func (c *Client) fail(err error) {
	c.once.Do(func() {
		c.err.Store(err)
		close(c.done)
		c.conn.Close()
	})
}

// Close logs out (best effort) and closes the transport.
func (c *Client) Close() error {
	select {
	case <-c.done:
		return nil
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = c.Request(ctx, protocol.CmdLogout, time.Second)
	c.fail(ErrClosed)
	return nil
}

func (c *Client) readLoop() {
	var dec frame.Decoder
	buf := make([]byte, 4096)
	for {
		n, err := c.conn.Read(buf)
		if n > 0 {
			dec.Write(buf[:n])
			for {
				f, ok := dec.Next()
				if !ok {
					break
				}
				msg := latin1(f.Payload)
				c.log.Debug("rx", "msg", msg)
				select {
				case c.rx <- msg:
				default: // nobody is reading; drop the oldest
					<-c.rx
					c.rx <- msg
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = ErrClosed
			}
			c.fail(err)
			return
		}
	}
}

// latin1 decodes controller text: point names use ISO-8859-1 (e.g. "Kühlen").
func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func (c *Client) keepAlive() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			if time.Since(time.Unix(0, c.lastTx.Load())) < keepAliveIdle {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := c.Login(ctx); err != nil {
				c.log.Warn("keep-alive login failed", "err", err)
			}
			cancel()
		}
	}
}

// exchange sends cmd and collects up to n replies (see collect).
func (c *Client) exchange(ctx context.Context, cmd string, n int, timeout time.Duration, raw bool) ([]string, error) {
	if err := c.send(cmd, raw); err != nil {
		return nil, err
	}
	return c.collect(ctx, cmd, n, timeout)
}

// send discards stale replies of earlier, timed-out exchanges and writes cmd,
// framed unless raw is set. Callers hold txMu.
func (c *Client) send(cmd string, raw bool) error {
	select {
	case <-c.done:
		return c.Err()
	default:
	}
	if c.readOnly.Load() && (raw || !protocol.IsReadCommand(cmd)) {
		return fmt.Errorf("%w: %q", ErrReadOnly, strings.TrimSpace(cmd))
	}
	for drained := false; !drained; {
		select {
		case <-c.rx:
		default:
			drained = true
		}
	}
	data := []byte(cmd)
	if !raw {
		data = frame.Encode(data, frame.AddrHeatPump, frame.AddrClient)
	}
	c.log.Debug("tx", "msg", strings.TrimSpace(cmd))
	c.lastTx.Store(time.Now().UnixNano())
	if _, err := c.conn.Write(data); err != nil {
		c.fail(err)
		return err
	}
	return nil
}

// collect waits for up to n replies. timeout is the maximum gap between
// replies; it restarts whenever one arrives. Getting fewer than n replies is
// not an error, getting none is.
func (c *Client) collect(ctx context.Context, cmd string, n int, timeout time.Duration) ([]string, error) {
	var out []string
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for len(out) < n {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-c.done:
			return out, c.Err()
		case msg := <-c.rx:
			out = append(out, msg)
			timer.Reset(timeout)
		case <-timer.C:
			if len(out) == 0 {
				return nil, fmt.Errorf("%w to %q", ErrTimeout, strings.TrimSuffix(cmd, ";"))
			}
			return out, nil
		}
	}
	return out, nil
}

// Request sends one command and returns the first reply.
func (c *Client) Request(ctx context.Context, cmd string, timeout time.Duration) (string, error) {
	c.txMu.Lock()
	defer c.txMu.Unlock()
	r, err := c.exchange(ctx, cmd, 1, timeout, false)
	if err != nil {
		return "", err
	}
	return r[0], nil
}

// RequestPrefix sends cmd and returns the first reply starting with prefix.
// Other replies are discarded: the controller keeps streaming the records of
// a counted request (~RM, ~PRL, ...) after an earlier exchange gave up, and
// those must not be mistaken for the answer.
func (c *Client) RequestPrefix(ctx context.Context, cmd, prefix string, timeout time.Duration) (string, error) {
	c.txMu.Lock()
	defer c.txMu.Unlock()
	return c.exchangePrefix(ctx, cmd, prefix, timeout)
}

func (c *Client) exchangePrefix(ctx context.Context, cmd, prefix string, timeout time.Duration) (string, error) {
	if err := c.send(cmd, false); err != nil {
		return "", err
	}
	deadline := time.Now().Add(3 * timeout)
	for {
		r, err := c.collect(ctx, cmd, 1, min(timeout, time.Until(deadline)))
		if err != nil {
			return "", err
		}
		msg := strings.TrimSpace(r[0])
		if strings.HasPrefix(msg, prefix) {
			return msg, nil
		}
		c.log.Debug("discarding unrelated reply", "cmd", cmd, "reply", msg)
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%w to %q (only unrelated replies)", ErrTimeout, strings.TrimSuffix(cmd, ";"))
		}
	}
}

// RequestMany sends one command and collects up to n replies.
func (c *Client) RequestMany(ctx context.Context, cmd string, n int, timeout time.Duration) ([]string, error) {
	c.txMu.Lock()
	defer c.txMu.Unlock()
	return c.exchange(ctx, cmd, n, timeout, false)
}

// requestCounted handles the "count, then N records" pattern of ~RM, ~RS and ~PRL.
func (c *Client) requestCounted(ctx context.Context, cmd string, first, gap time.Duration, progress func(done, total int)) (int, []string, error) {
	c.txMu.Lock()
	defer c.txMu.Unlock()
	head, err := c.exchangePrefix(ctx, cmd, "~SUM", first)
	if err != nil {
		return 0, nil, err
	}
	total, err := protocol.ParseCount(head)
	if err != nil {
		return 0, nil, err
	}
	var out []string
	for len(out) < total {
		batch, err := c.collect(ctx, cmd, 1, gap)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				break
			}
			return total, out, err
		}
		out = append(out, batch...)
		if progress != nil {
			progress(len(out), total)
		}
	}
	return total, out, nil
}

// Login sends ~LIN. If the controller is silent, a modem CONNECT line is sent
// first (controllers behind an analog/GSM modem need it) and the login retried.
func (c *Client) Login(ctx context.Context) error {
	c.txMu.Lock()
	defer c.txMu.Unlock()
	_, err := c.exchangePrefix(ctx, protocol.CmdLogin, "~OK", time.Second)
	if errors.Is(err, ErrTimeout) && !c.readOnly.Load() {
		_, _ = c.exchange(ctx, protocol.ModemConnect, 0, 0, true)
		_, err = c.exchangePrefix(ctx, protocol.CmdLogin, "~OK", time.Second)
	}
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return nil
}
