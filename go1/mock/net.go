//
// Written by Maxim Khitrov (June 2013)
//

package mock

import (
	"io"
	"net"
	"sync"
	"time"
)

// Conn is an in-memory implementation of net.Conn.
type Conn struct {
	mu *sync.Mutex   // Control mutex
	r  *halfConn     // Read side
	w  *halfConn     // Write side
	rd time.Time     // Read deadline
	wd time.Time     // Write deadline
	t  time.Duration // Read/write timeout
}

// NewConn creates a pair of connected net.Conn instances.
func NewConn(addrA, addrB string, bufSize int) (*Conn, *Conn) {
	if bufSize <= 0 {
		bufSize = 4096
	}
	mu := new(sync.Mutex)
	a := newHalfConn(mu, addrA, bufSize)
	b := newHalfConn(mu, addrB, bufSize)
	return &Conn{mu: mu, r: a, w: b}, &Conn{mu: mu, r: b, w: a}
}

func (c *Conn) Read(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n, err = c.r.read(b, c.rd, c.t, c.r.addr); err == io.EOF {
		c.close()
	}
	return
}

func (c *Conn) Write(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.w.write(b, c.wd, c.t, c.r.addr)
}

func (c *Conn) LocalAddr() net.Addr  { return netAddr(c.r.addr) }
func (c *Conn) RemoteAddr() net.Addr { return netAddr(c.w.addr) }

func (c *Conn) SetDeadline(t time.Time) error      { return c.deadline(t, true, true) }
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.deadline(t, true, false) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.deadline(t, false, true) }
func (c *Conn) SetTimeout(t time.Duration)         { c.mu.Lock(); c.t = t; c.mu.Unlock() }

func (c *Conn) Clear() {
	c.mu.Lock()
	c.r.buf = c.r.buf[:0]
	c.w.buf = c.w.buf[:0]
	c.r.off = 0
	c.w.off = 0
	c.r.Broadcast()
	c.w.Broadcast()
	c.mu.Unlock()
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.close()
	return nil
}

func (c *Conn) deadline(t time.Time, r, w bool) error {
	c.mu.Lock()
	if r {
		c.rd = t
	}
	if w {
		c.wd = t
	}
	c.mu.Unlock()
	return nil
}

func (c *Conn) close() {
	if c.r.buf != nil {
		c.r.buf = nil
		c.r.eof = true
		c.w.eof = true
		c.r.Broadcast()
		c.w.Broadcast()
	}
}

type halfConn struct {
	sync.Cond

	addr string // Reader's address
	buf  []byte // Read/write buffer
	off  int    // Read offset in buf
	eof  bool   // Writer closed
}

func newHalfConn(mu *sync.Mutex, addr string, bufSize int) *halfConn {
	return &halfConn{
		Cond: *sync.NewCond(mu),
		addr: addr,
		buf:  make([]byte, 0, bufSize),
	}
}

func (c *halfConn) read(b []byte, d time.Time, t time.Duration, addr string) (n int, err error) {
	var alarm netTimer
	alarm.Set(d, t)
	for {
		switch {
		case c.buf == nil:
			return n, io.EOF
		case alarm.Timeout():
			return n, netTimeout("mock(" + addr + "): read timeout")
		case len(b) == 0:
			return n, nil
		case len(c.buf) > 0:
			n = copy(b, c.buf[c.off:])
			if c.off += n; c.off == len(c.buf) {
				c.buf = c.buf[:0]
				c.off = 0
				if c.eof {
					err = io.EOF
				}
			}
			c.Broadcast()
			return n, err
		}
		if alarm.Schedule(&c.Cond) {
			defer alarm.Stop()
		}
		c.Wait()
	}
}

func (c *halfConn) write(b []byte, d time.Time, t time.Duration, addr string) (n int, err error) {
	var alarm netTimer
	alarm.Set(d, t)
	for {
		switch {
		case c.eof:
			return n, io.EOF
		case alarm.Timeout():
			return n, netTimeout("mock(" + addr + "): write timeout")
		case len(b) == 0:
			return n, err
		case len(c.buf)-c.off < cap(c.buf):
			if len(c.buf) == cap(c.buf) {
				c.buf = c.buf[:copy(c.buf, c.buf[c.off:])]
				c.off = 0
			}
			i := copy(c.buf[len(c.buf):cap(c.buf)], b[n:])
			c.buf = c.buf[:len(c.buf)+i]
			c.Broadcast()
			if n += i; n == len(b) {
				return n, err
			}
		}
		if alarm.Schedule(&c.Cond) {
			defer alarm.Stop()
		}
		c.Wait()
	}
}

// netTimer interrupts blocked Read/Write calls when the timeout expires.
type netTimer struct {
	*time.Timer
	now, end time.Time
}

// Set configures the timer for the deadline or timeout, whichever is earlier.
func (t *netTimer) Set(deadline time.Time, timeout time.Duration) {
	if timeout > 0 {
		t.end = time.Now().Add(timeout)
		if !deadline.IsZero() && deadline.Before(t.end) {
			t.end = deadline
		}
	} else if !deadline.IsZero() {
		t.end = deadline
	}
}

// Timeout returns true if the deadline has passed. This method must be called
// before the first call to Schedule.
func (t *netTimer) Timeout() bool {
	if !t.end.IsZero() {
		t.now = time.Now()
		return !t.end.After(t.now)
	}
	return false
}

// Schedule configures the timer to call c.Broadcast() when the deadline has
// passed. It returns true if the caller should defer a call to t.Stop().
func (t *netTimer) Schedule(c *sync.Cond) (deferStop bool) {
	if !t.end.IsZero() {
		d := t.end.Sub(t.now)
		if t.Timer == nil {
			t.Timer = time.AfterFunc(d, func() { c.Broadcast() })
			return true
		}
		t.Reset(d)
	}
	return false
}

// netAddr implements net.Addr for the "mock" network.
type netAddr string

func (a netAddr) Network() string { return "mock" }
func (a netAddr) String() string  { return string(a) }

// netTimeout implements net.Error that always indicates a timeout.
type netTimeout string

func (e netTimeout) Error() string   { return string(e) }
func (e netTimeout) Timeout() bool   { return true }
func (e netTimeout) Temporary() bool { return false }
