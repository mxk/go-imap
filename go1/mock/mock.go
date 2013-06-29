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

// connTimeout is the Read/Write timeout for the client and server connections.
const connTimeout = 500 * time.Millisecond

// ScriptFunc is a function executed as part of the server's script.
type ScriptFunc func(s imap.MockServer) error

var (
	STARTTLS ScriptFunc = func(s imap.MockServer) error { return s.EnableTLS(serverTLS()) }
	DEFLATE  ScriptFunc = func(s imap.MockServer) error { return s.EnableDeflate(-1) }
	CLOSE    ScriptFunc = func(s imap.MockServer) error { return s.Close(true) }
)

type T struct {
	*testing.T

	s  imap.MockServer
	c  *imap.Client
	cn net.Conn
	ch <-chan interface{}
}

// Server launches a new scripted server.
func Server(t *testing.T, script ...interface{}) *T {
	c, s := NewConn("client", "server", 0)
	c.SetTimeout(connTimeout)
	s.SetTimeout(connTimeout)
	mt := &T{T: t, s: imap.NewMockServer(s), cn: c}
	mt.Script(script...)
	return mt
}

// Dial returns a new Client instance.
func (t *T) Dial() (*imap.Client, error) {
	cn := t.cn
	if t.cn = nil; cn == nil {
		panic("mock: Dial already called")
	}
	var err error
	t.c, err = imap.NewClient(cn, ServerName, connTimeout)
	return t.c, err
}

// Script runs the server script in a separate goroutine.
func (t *T) Script(script ...interface{}) {
	select {
	case <-t.ch:
		t.ch = nil
	default:
		if t.ch != nil {
			t.Fatal(cl("t.Script() another script is still running"))
		}
	}
	ch := make(chan interface{}, 1)
	t.ch = ch
	go t.script(script, ch)
}

func (t *T) script(script []interface{}, ch chan<- interface{}) {
	defer func() { ch <- recover(); close(ch) }()
	var who string
	var err error
	for ln, v := range script {
		ln++
		switch v := v.(type) {
		case string:
			if len(v) < 3 || v[:3] != "S: " && v[:3] != "C: " {
				panicf(`%+q (line %d) does not begin with "S: " or "C: "`, v, ln)
			}
			if who, v = v[:1], v[3:]; len(v) == 0 {
				if ln < len(script) {
					if _, ok := script[ln].([]byte); ok {
						continue
					}
				}
				panicf(`%+q (line %d) is not followed by a []byte`, v, ln)
			}
			if who == "S" {
				if err = t.s.WriteLine([]byte(v)); err == nil {
					err = t.s.Flush()
				}
			} else {
				var b []byte
				if b, err = t.s.ReadLine(); string(b) != v {
					panicf("s.ReadLine() expected %+q; got %+q (%v)", v, b, err)
				}
			}
		case []byte:
			// TODO: Is this actually needed?
			if who == "S" {
				if _, err = t.s.Write(v); err == nil {
					err = t.s.Flush()
				}
			} else if who == "C" {
				b := make([]byte, len(v))
				if _, err = io.ReadFull(t.s, b); string(b) != string(v) {
					panicf("s.Read() expected %+q; got %+q (%v)", v, b, err)
				}
			} else {
				panicf(`%+q (line %d) was not preceeded by "S: " or "C: "`, v, ln)
			}
		case ScriptFunc:
			err = v(t.s)
		default:
			panicf("%#v (line %d) is not a valid script action", v, ln)
		}
		if err != nil {
			panic(err)
		}
		who = ""
	}
}

// Join waits for script completion and reports the errors returned by the
// server and client.
func (t *T) Join(err error) {
	if err, ok := <-t.ch; err != nil || !ok {
		if ok {
			t.Errorf(cl("Join() server: %v"), err)
		} else {
			t.Errorf(cl("Join() called without an active script"))
		}
	}
	if err != nil {
		t.Errorf(cl("Join() client: %v"), err)
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

func cl(s string) string {
	_, testFile, line, ok := runtime.Caller(2)
	if ok && strings.HasSuffix(testFile, "_test.go") {
		return fmt.Sprintf("%d: %s", line, s)
	}
	return s
}

func panicf(format string, v ...interface{}) {
	panic(fmt.Sprintf(format, v...))
}
