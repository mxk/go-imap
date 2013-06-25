//
// Written by Maxim Khitrov (June 2013)
//

package mock_test

import (
	"testing"

	"code.google.com/p/go-imap/go1/imap"
	"code.google.com/p/go-imap/go1/mock"
)

func TestNewClientOK(T *testing.T) {
	C, t := mock.Client(T,
		`S: * OK Test server ready`,
		`C: A1 CAPABILITY`,
		`S: * CAPABILITY IMAP4rev1 XYZZY`,
		`S: A1 OK Thats all she wrote!`,
		mock.EOF,
	)
	t.CheckState(imap.Login)
	t.CheckCaps("IMAP4rev1", "XYZZY")
	t.WaitEOF()

	if len(C.Data) != 1 || C.Data[0].Info != "Test server ready" {
		t.Errorf("C.Data expected greeting; got %v", C.Data)
	}
}
