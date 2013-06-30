//
// Written by Maxim Khitrov (June 2013)
//

package mock

import (
	"fmt"
	"io"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"code.google.com/p/go-imap/go1/imap"
)

// ServerName is the hostname used by the scripted server.
var ServerName = "imap.mock.net"

// Timeout is the time limit used for each Read and Write call by the client and
// the server.
var Timeout = 500 * time.Millisecond

// Send and Recv are script actions for sending and receiving raw data. They can
// be used for transferring multi-line literals.
type (
	Send []byte
	Recv []byte
)

// ScriptFunc is function type called during script execution to control the
// server state. STARTTLS, DEFLATE, and CLOSE are predefined script actions for
// the most common operations.
type ScriptFunc func(s imap.MockServer) error

// Script actions that affect server state.
var (
	STARTTLS = func(s imap.MockServer) error { return s.EnableTLS(serverTLS()) }
	DEFLATE  = func(s imap.MockServer) error { return s.EnableDeflate(-1) }
	CLOSE    = func(s imap.MockServer) error { return s.Close(true) }
)

// T wraps existing test state and provides methods for testing the IMAP client
// against a scripted server.
type T struct {
	*testing.T

	s  imap.MockServer
	c  *imap.Client
	cn net.Conn
	ch <-chan interface{}
}

// Server launches a new scripted server that can handle one client connection.
func Server(t *testing.T, script ...interface{}) *T {
	c, s := NewConn("client", "server", 0)
	c.SetTimeout(Timeout)
	s.SetTimeout(Timeout)
	mt := &T{T: t, s: imap.NewMockServer(s), cn: c}
	mt.Script(script...)
	return mt
}

// Dial returns a new Client connected to the scripted server. This method may
// not be called more than once for each server instance.
func (t *T) Dial() (*imap.Client, error) {
	cn := t.cn
	if t.cn = nil; cn == nil {
		panic("mock: Dial already called")
	}
	var err error
	t.c, err = imap.NewClient(cn, ServerName, Timeout)
	return t.c, err
}

// Script runs a server script in a separate goroutine. A script is a sequence
// of string, Send, Recv, and ScriptFunc actions. Strings represent lines of
// text to be sent ("S: ...") or received ("C: ...") by the server. There is an
// implicit CRLF at the end of each line. Send and Recv allow the server to send
// and receive raw bytes (usually multi-line literals). ScriptFunc allows server
// state changes by calling methods on the provided imap.MockServer instance.
func (t *T) Script(script ...interface{}) {
	select {
	case <-t.ch:
	default:
		if t.ch != nil {
			t.Fatal(cl("t.Script() called while another script is active"))
		}
	}
	ch := make(chan interface{}, 1)
	t.ch = ch
	go t.script(script, ch)
}

// Join waits for script completion and reports the errors encountered by the
// client and the server.
func (t *T) Join(err error) {
	if err, ok := <-t.ch; err != nil {
		t.Errorf(cl("t.Join() S: %v"), err)
	} else if !ok {
		t.Fatalf(cl("t.Join() called without an active script"))
	}
	if err != nil {
		t.Errorf(cl("t.Join() C: %v"), err)
	}
	if t.Failed() {
		t.FailNow()
	}
}

// StartTLS performs client-side TLS negotiation. It should be used in
// combination with the STARTTLS script action.
func (t *T) StartTLS() error {
	_, err := t.c.StartTLS(clientTLS())
	return err
}

// script runs the provided script and sends the first encountered error to ch,
// which is then closed.
func (t *T) script(script []interface{}, ch chan<- interface{}) {
	defer func() { ch <- recover(); close(ch) }()
	for ln, v := range script {
		switch ln++; v := v.(type) {
		case string:
			if strings.HasPrefix(v, "S: ") {
				t.flush(ln, v, t.s.WriteLine([]byte(v[3:])))
			} else if strings.HasPrefix(v, "C: ") {
				b, err := t.s.ReadLine()
				t.compare(ln, v[3:], string(b), err)
			} else {
				panicf(`[#%d] %+q must be prefixed with "S: " or "C: "`, ln, v)
			}
		case Send:
			_, err := t.s.Write(v)
			t.flush(ln, v, err)
		case Recv:
			b := make([]byte, len(v))
			_, err := io.ReadFull(t.s, b)
			t.compare(ln, string(v), string(b), err)
		case ScriptFunc:
			t.run(ln, v)
		case func(s imap.MockServer) error:
			t.run(ln, v)
		default:
			panicf("[#%d] %T is not a valid script action", ln, v)
		}
	}
}

// flush sends any buffered data to the client and panics if there is an error.
func (t *T) flush(ln int, v interface{}, err error) {
	if err == nil {
		err = t.s.Flush()
	}
	if err != nil {
		panicf("[#%d] %+q write error: %v", ln, v, err)
	}
}

// compare panics if v != b or err != nil.
func (t *T) compare(ln int, v, b string, err error) {
	if v != b || err != nil {
		panicf("[#%d] expected %+q; got %+q (%v)", ln, v, b, err)
	}
}

// run calls f and panics if it returns an error.
func (t *T) run(ln int, f ScriptFunc) {
	if err := f(t.s); err != nil {
		panicf("[#%d] ScriptFunc error: %v", ln, err)
	}
}

// cl prefixes s with the current line number in the calling test function.
func cl(s string) string {
	_, testFile, line, ok := runtime.Caller(2)
	if ok && strings.HasSuffix(testFile, "_test.go") {
		return fmt.Sprintf("%d: %s", line, s)
	}
	return s
}

// panicf must be documented for consistency (you're welcome)!
func panicf(format string, v ...interface{}) {
	panic(fmt.Sprintf(format, v...))
}
