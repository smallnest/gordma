package rdmanet

import (
	"sync"
	"testing"
	"time"

	"github.com/smallnest/gordma/handshake"
)

// TestHandshakeExchangeRoundTrip exercises the TCP out-of-band exchange that the
// handshake-based Dial/Listen rely on, end to end over a loopback socket. It is
// pure Go (no RDMA hardware): it verifies that the endpoint info each side
// constructs is delivered intact to the peer, which is the wire contract the
// rdmanet handshake path depends on.
func TestHandshakeExchangeRoundTrip(t *testing.T) {
	srv, err := handshake.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("handshake.Listen: %v", err)
	}
	defer srv.Close()

	serverLocal := handshake.EndpointInfo{
		QPN: 0x111, PSN: 0xaaa, LID: 1, GIDIndex: 3,
		RKey: 0x1234, RemoteAddr: 0xdeadbeef,
	}
	clientLocal := handshake.EndpointInfo{
		QPN: 0x222, PSN: 0xbbb, LID: 2, GIDIndex: 3,
		RKey: 0x5678, RemoteAddr: 0xcafef00d,
	}

	var (
		wg          sync.WaitGroup
		gotByServer handshake.EndpointInfo
		serverErr   error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		gotByServer, serverErr = srv.Accept(serverLocal, 2*time.Second)
	}()

	gotByClient, err := handshake.Dial(srv.Addr().String(), clientLocal, 2*time.Second)
	if err != nil {
		t.Fatalf("handshake.Dial: %v", err)
	}
	wg.Wait()
	if serverErr != nil {
		t.Fatalf("server Accept: %v", serverErr)
	}

	// The server must observe the client's info and vice versa.
	if gotByServer.QPN != clientLocal.QPN || gotByServer.RKey != clientLocal.RKey {
		t.Errorf("server saw %+v, want client's %+v", gotByServer, clientLocal)
	}
	if gotByClient.QPN != serverLocal.QPN || gotByClient.RemoteAddr != serverLocal.RemoteAddr {
		t.Errorf("client saw %+v, want server's %+v", gotByClient, serverLocal)
	}
}
