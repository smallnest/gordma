package handshake

import (
	"testing"
	"time"
)

// TestExchangeRoundTrip runs a real server and client over loopback TCP and
// verifies both sides receive each other's EndpointInfo intact. This needs no
// RDMA hardware — it exercises the full out-of-band protocol.
func TestExchangeRoundTrip(t *testing.T) {
	srv, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Close()

	serverInfo := EndpointInfo{
		QPN: 0x111, PSN: 0xabc, LID: 7, GIDIndex: 3,
		RKey: 0xdead, RemoteAddr: 0x1000,
	}
	serverInfo.GID[0] = 0xfe
	serverInfo.GID[1] = 0x80

	clientInfo := EndpointInfo{
		QPN: 0x222, PSN: 0xdef, LID: 9, GIDIndex: 3,
		RKey: 0xbeef, RemoteAddr: 0x2000,
	}
	clientInfo.GID[0] = 0xfe
	clientInfo.GID[15] = 0x02

	type result struct {
		peer EndpointInfo
		err  error
	}
	srvCh := make(chan result, 1)
	go func() {
		peer, err := srv.Accept(serverInfo, 3*time.Second)
		srvCh <- result{peer, err}
	}()

	gotServer, err := Dial(srv.Addr().String(), clientInfo, 3*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	sr := <-srvCh
	if sr.err != nil {
		t.Fatalf("Accept: %v", sr.err)
	}

	// Client should have received the server's info.
	if gotServer != serverInfo {
		t.Errorf("client received %+v, want %+v", gotServer, serverInfo)
	}
	// Server should have received the client's info.
	if sr.peer != clientInfo {
		t.Errorf("server received %+v, want %+v", sr.peer, clientInfo)
	}
}

// TestDialTimeout verifies Dial fails fast against a closed port.
func TestDialTimeout(t *testing.T) {
	// 127.0.0.1:1 is reserved/unused; connection should fail quickly.
	_, err := Dial("127.0.0.1:1", EndpointInfo{}, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected error dialing closed port")
	}
}
