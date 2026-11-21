package session

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"tsps-go/internal/protocol"
)

func wsURL(t *testing.T, srv *httptest.Server, path string) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	u.Scheme = "ws"
	u.Path = path
	return u.String()
}

func TestHandlerNonWSGets404(t *testing.T) {
	srv := httptest.NewServer(NewHandler(func(ch Channel) *Connection { return NewConnection(ch, nil, nil, 600) }, nil))
	defer srv.Close()
	for _, path := range []string{"/", "/api/world-info", "/gateway/jav_config.ws", "/regions/123.pack"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("GET %s CORS header = %q, want *", path, got)
		}
	}
}

func TestHandlerWSRoundTrip(t *testing.T) {
	addrCh := make(chan string, 1)
	connCh := make(chan *Connection, 1)
	srv := httptest.NewServer(NewHandler(func(ch Channel) *Connection {
		addrCh <- ch.RemoteAddr()
		c := NewConnection(ch, nil, nil, 600)
		connCh <- c
		return c
	}, nil))
	defer srv.Close()

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	// Any path upgrades (no path restriction in TS).
	conn, _, err := dialer.Dial(wsURL(t, srv, "/some/path"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	gotAddr := <-addrCh
	gotConn := <-connCh

	// The welcome packet arrives immediately: opcode 0 + tickMs + time.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	mt, welcome, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("welcome read: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Errorf("welcome message type = %d, want binary", mt)
	}
	if len(welcome) != 9 || welcome[0] != protocol.ServerWelcome {
		t.Errorf("welcome = %v, want 9 bytes starting with opcode 0", welcome)
	}
	if gotAddr == "" {
		t.Error("no remote address captured")
	}
	if gotConn == nil || gotConn.IsClosed() {
		t.Error("connection missing or closed after welcome")
	}

	// A ping frame is accepted (no reply expected).
	if err := conn.WriteMessage(websocket.BinaryMessage, clientFrame(protocol.ClientPing, pingPayload(1))); err != nil {
		t.Fatalf("ping write: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if gotConn.IsClosed() {
		t.Error("connection closed after ping")
	}

	// A malformed frame closes with 1002.
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0}); err != nil {
		t.Fatalf("malformed write: %v", err)
	}
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatal("expected close after malformed frame")
	}
	if !websocket.IsCloseError(err, 1002) {
		t.Errorf("close error = %v, want 1002", err)
	}
}

func TestHandlerXRealIPFromLoopback(t *testing.T) {
	addrCh := make(chan string, 2)
	srv := httptest.NewServer(NewHandler(func(ch Channel) *Connection {
		addrCh <- ch.RemoteAddr()
		return NewConnection(ch, nil, nil, 600)
	}, nil))
	defer srv.Close()

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	// httptest clients connect from loopback, so x-real-ip is honored.
	headers := http.Header{"X-Real-IP": []string{"203.0.113.7"}}
	conn, _, err := dialer.Dial(wsURL(t, srv, "/"), headers)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if gotAddr := <-addrCh; gotAddr != "203.0.113.7" {
		t.Errorf("remote addr with x-real-ip = %q, want 203.0.113.7", gotAddr)
	}

	// An invalid x-real-ip value falls back to the peer address.
	headers = http.Header{"X-Real-IP": []string{"not-an-ip"}}
	conn2, _, err := dialer.Dial(wsURL(t, srv, "/"), headers)
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	defer conn2.Close()
	if gotAddr := <-addrCh; gotAddr == "not-an-ip" || gotAddr == "" {
		t.Errorf("remote addr with invalid x-real-ip = %q", gotAddr)
	}
}

func TestHandlerNilConnection(t *testing.T) {
	srv := httptest.NewServer(NewHandler(func(ch Channel) *Connection { return nil }, nil))
	defer srv.Close()
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL(t, srv, "/"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	if err == nil || !websocket.IsCloseError(err, 1011) {
		t.Errorf("read after nil connection = %v, want close 1011", err)
	}
}

func gatewayTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	t.Setenv("PUBLIC_HOST", "")
	gateway := NewGatewayTarget(43594, 242, true, 0, "", nil)
	return httptest.NewServer(NewHandler(func(ch Channel) *Connection { return NewConnection(ch, nil, nil, 600) }, gateway))
}

func getPath(t *testing.T, srv *httptest.Server, path string) (int, http.Header, []byte) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, body
}

func TestHandlerGatewayServed(t *testing.T) {
	srv := gatewayTestServer(t)
	defer srv.Close()

	status, header, body := getPath(t, srv, "/gateway/jav_config.ws")
	if status != http.StatusOK {
		t.Fatalf("GET jav_config = %d, want 200", status)
	}
	if got := header.Get("Content-Type"); got != "text/plain; charset=iso-8859-1" {
		t.Errorf("jav_config Content-Type = %q, want text/plain; charset=iso-8859-1", got)
	}
	if got := header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("jav_config CORS header = %q, want *", got)
	}
	if string(body) != expectedJavConfig("127.0.0.1", 43594, 242) {
		t.Errorf("jav_config body mismatch: %q", body)
	}

	status, header, body = getPath(t, srv, "/gateway/worldlist.ws")
	if status != http.StatusOK {
		t.Fatalf("GET worldlist = %d, want 200", status)
	}
	if got := header.Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("worldlist Content-Type = %q, want application/octet-stream", got)
	}
	if got := header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("worldlist CORS header = %q, want *", got)
	}
	if got := decodeWorldList(t, body); got.id != 1 || got.count != 1 || got.flags != 0x1 || got.host != "127.0.0.1" || got.activity != "tsps" {
		t.Errorf("worldlist body = %+v", got)
	}
}

func TestHandlerGatewayQueryStringsAndMethods(t *testing.T) {
	srv := gatewayTestServer(t)
	defer srv.Close()

	// Query strings are stripped before the exact-path match.
	for _, path := range []string{"/gateway/jav_config.ws?x=1", "/gateway/worldlist.ws?foo=bar"} {
		status, _, _ := getPath(t, srv, path)
		if status != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, status)
		}
	}

	// No method check on gateway paths (TS checks method only for
	// ContentApi/browser-host, not for GatewayTarget.resolve).
	resp, err := http.Post(srv.URL+"/gateway/jav_config.ws", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST jav_config = %d, want 200", resp.StatusCode)
	}
}

func TestHandlerGatewayUnknownPaths404(t *testing.T) {
	srv := gatewayTestServer(t)
	defer srv.Close()
	for _, path := range []string{
		"/",
		"/gateway",
		"/gateway/jav_config.ws/extra",
		"/gateway/worldlist.ws/extra",
		"/gateway%2Fjav_config.ws",
		"/api/world-info",
		"/regions",
	} {
		status, header, _ := getPath(t, srv, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, status)
		}
		if got := header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("GET %s CORS header = %q, want *", path, got)
		}
	}
}

func TestHandlerNilGatewayStill404s(t *testing.T) {
	srv := httptest.NewServer(NewHandler(func(ch Channel) *Connection { return NewConnection(ch, nil, nil, 600) }, nil))
	defer srv.Close()
	for _, path := range []string{"/gateway/jav_config.ws", "/gateway/worldlist.ws"} {
		status, _, _ := getPath(t, srv, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s with nil gateway = %d, want 404", path, status)
		}
	}
}

func TestHandlerGatewayWSUpgradeUnaffected(t *testing.T) {
	srv := gatewayTestServer(t)
	defer srv.Close()
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL(t, srv, "/some/path"), nil)
	if err != nil {
		t.Fatalf("dial with gateway present: %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	mt, welcome, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("welcome read: %v", err)
	}
	if mt != websocket.BinaryMessage || len(welcome) != 9 || welcome[0] != protocol.ServerWelcome {
		t.Errorf("welcome = type %d %v, want binary 9 bytes starting with opcode 0", mt, welcome)
	}
}
