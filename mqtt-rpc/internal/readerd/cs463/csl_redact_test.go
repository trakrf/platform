package cs463

import (
	"context"
	"net"
	"strings"
	"testing"
)

// closedURL returns a base URL nothing listens on, so a request fails in transport.
func closedURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return "http://" + addr
}

// A transport failure must not carry the request URL: the sessionless commands put
// the password in the query string, and these errors reach the journal and the
// cloud as RPC error messages.
func TestDo_TransportErrorOmitsPassword(t *testing.T) {
	const secret = "s3cr3t-pass"
	c := New(closedURL(t), "trakrf", secret, 0)

	err := c.DirectIOOutput(context.Background(), 1, true)
	if err == nil {
		t.Fatal("DirectIOOutput against a closed port: want an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks the password: %v", err)
	}
}
