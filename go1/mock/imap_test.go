//
// Written by Maxim Khitrov (June 2013)
//

package mock

import (
	"testing"

	"code.google.com/p/go-imap/go1/imap"
)

func TestNewClientOK(T *testing.T) {
	C, t := Client(T,
		`S: * OK Test server ready`,
		`C: A1 CAPABILITY`,
		`S: * CAPABILITY IMAP4rev1 XYZZY`,
		`S: A1 OK Thats all she wrote!`,
		EOF,
	)
	t.CheckState(imap.Login)
	t.CheckCaps("IMAP4rev1", "XYZZY")
	t.WaitEOF()

	if len(C.Data) != 1 || C.Data[0].Info != "Test server ready" {
		t.Errorf("C.Data expected greeting; got %v", C.Data)
	}
}
