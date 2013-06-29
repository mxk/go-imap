//
// Written by Maxim Khitrov (June 2013)
//

package mock_test

import (
	"testing"

	"code.google.com/p/go-imap/go1/imap"
	"code.google.com/p/go-imap/go1/mock"
)

func setLogMask(mask imap.LogMask) func() {
	imap.DefaultLogMask = mask
	return func() { imap.DefaultLogMask = imap.LogNone }
}

func un(f func()) {
	f()
}

func TestServer(T *testing.T) {
	defer un(setLogMask(imap.LogAll))

	// Connect
	t := mock.Server(T,
		`S: * OK Mock server ready!`,
		`C: A1 CAPABILITY`,
		`S: * CAPABILITY IMAP4rev1 LOGINDISABLED STARTTLS`,
		`S: A1 OK Thats all she wrote!`,
	)
	c, err := t.Dial()
	t.Join(err)
	if len(c.Data) != 1 || c.Data[0].Info != "Mock server ready!" {
		t.Fatalf("c.Data expected greeting; got %v", c.Data)
	}

	// StartTLS
	t.Script(
		`C: A2 STARTTLS`,
		`S: A2 OK Begin TLS negotiation now`,
		mock.STARTTLS,
		`C: A3 CAPABILITY`,
		`S: * CAPABILITY IMAP4rev1`,
		`S: A3 OK Thats all she wrote!`,
	)
	t.Join(t.StartTLS())

	// Login
	t.Script(
		`C: A4 LOGIN {11}`,
		`S: + Ready for additional command text`,
		`C: FRED FOOBAR {7}`,
		`S: + Ready for additional command text`,
		`C: fat man`,
		`S: A4 OK LOGIN completed`,
	)
	user := imap.NewLiteral([]byte("FRED FOOBAR"))
	pass := imap.NewLiteral([]byte("fat man"))
	_, err = imap.Wait(c.Send("LOGIN", user, pass))
	t.Join(err)
}
