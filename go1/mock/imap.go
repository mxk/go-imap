//
// Written by Maxim Khitrov (June 2013)
//

package mock

import (
	"fmt"
	"io"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"code.google.com/p/go-imap/go1/imap"
)

const (
	block = time.Duration(-1)
	poll  = time.Duration(0)
)

type ScriptFunc func(S imap.MockServer) error

var (
	STARTTLS ScriptFunc = func(S imap.MockServer) error { return S.StartTLS(nil) }
	DEFLATE  ScriptFunc = func(S imap.MockServer) error { return S.EnableDeflate(6) }
	EOF      ScriptFunc = func(S imap.MockServer) error { return S.Close(true) }
)

type T struct {
	*testing.T

	C   *imap.Client
	S   imap.MockServer
	err chan interface{}
}

func Client(test *testing.T, script ...interface{}) (*imap.Client, *T) {
	c, s := NewConn("client", "server", 0)
	t := &T{
		T:   test,
		S:   imap.NewMockServer(s),
		err: make(chan interface{}, 1),
	}

	var err error
	go t.Script(script...)
	t.C, err = imap.NewClient(c, "mock.net", 1*time.Second)
	t.Join("NewClient", err)
	return t.C, t
}

func (t *T) Script(script ...interface{}) {
	defer func() { t.err <- recover() }()
	var who string
	var err error
	S := t.S
	for _, act := range script {
		switch act := act.(type) {
		case ScriptFunc:
			act(t.S)
		case string:
			send := strings.HasPrefix(act, "S: ")
			if !send && !strings.HasPrefix(act, "C: ") {
				panicf("bad script line: %+q", act)
			}
			if act = act[3:]; send {
				if err = S.WriteLine([]byte(act)); err == nil {
					err = S.Flush()
				}
				who = "S"
			} else {
				var b []byte
				if b, err = S.ReadLine(); string(b) != act {
					panicf("S.ReadLine() expected %+q; got %+q (%v)", act, b, err)
				}
				who = "C"
			}
		case []byte:
			if who == "S" {
				if _, err = S.Write(act); err == nil {
					err = S.Flush()
				}
			} else if who == "C" {
				n := len(act)
				b := make([]byte, n)
				if n, err = io.ReadFull(S, b); string(b) != string(act) {
					panicf("S.Read() expected %+q; got %+q (%v)", act, b, err)
				}
			} else {
				panic("unexpected []byte")
			}
		case imap.Literal:
			panic("TODO")
		default:
			panicf("unexpected %T", act)
		}
		if err != nil {
			panic(err)
		}
	}
}

func (t *T) Join(checkpoint string, err error) {
	select {
	case err, ok := <-t.err:
		if !ok {
			t.Errorf(cl("%s (server): t.err is closed"), checkpoint)
		} else if err != nil {
			t.Errorf(cl("%s (server): %v"), checkpoint, err)
		}
	case <-time.After(5 * time.Second):
		t.Errorf(cl("%s (server): t.err timeout"), checkpoint)
	}
	if err != nil {
		t.Errorf(cl("%s (client): %v"), checkpoint, err)
	}
	if t.Failed() {
		t.FailNow()
	}
}

func (t *T) CheckState(want imap.ConnState) {
	if have := t.C.State(); have != want {
		t.Fatalf(cl("C.State() expected %v; got %v"), want, have)
	}
}

func (t *T) CheckCaps(want ...string) {
	have := make([]string, 0, len(t.C.Caps))
	for v := range t.C.Caps {
		have = append(have, v)
	}
	for i := range want {
		want[i] = strings.ToUpper(want[i])
	}
	sort.Strings(have)
	sort.Strings(want)
	if !reflect.DeepEqual(have, want) {
		t.Fatalf(cl("C.Caps expected %v; got %v"), want, have)
	}
}

func (t *T) WaitEOF() {
	if err := t.C.Recv(block); err != io.EOF {
		t.Fatalf(cl("C.Recv() expected EOF; got %v"), err)
	}
	t.CheckState(imap.Closed)
	if err := t.C.Recv(poll); err != io.EOF {
		t.Fatalf(cl("C.Recv() expected EOF; got %v"), err)
	}
}

func cl(s string) string {
	_, thisFile, _, _ := runtime.Caller(1)
	_, testFile, line, ok := runtime.Caller(2)
	if ok && thisFile == testFile {
		return fmt.Sprintf("%d: %s", line, s)
	}
	return s
}

func panicf(format string, v ...interface{}) {
	panic(fmt.Sprintf(format, v...))
}
