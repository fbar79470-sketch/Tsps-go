package session

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"tsps-go/internal/config"
)

// Channel is the transport abstraction, porting BinaryChannel
// (BinaryChannel.ts:10-21) in its Go shape: kinds "websocket" and
// "headless" only. The "webrtc" and "tcp" kinds exist in TS but are NOT
// implemented in this run (no WebRTC stack; no TCP listener opens a
// TcpBinaryChannel in TS src/ either).
//
// Unlike TS, which is callback-driven (onData/onClose/onError), Go code
// reads frames synchronously: the HTTP handler drives WebSocketChannel
// reads, and connection teardown is explicit via Connection.Close /
// Connection.Cleanup. Send is safe for concurrent use.
type Channel interface {
	// Kind reports "websocket" or "headless".
	Kind() string
	// RemoteAddr is the peer identity used for IP bans and logging.
	RemoteAddr() string
	// Send writes one binary message. It returns an error if the
	// transport is closed or the write fails.
	Send(payload []byte) error
	// Close terminates the transport with a WebSocket close code and
	// reason. It is idempotent.
	Close(code int, reason string)
	// IsOpen reports whether the transport can still send.
	IsOpen() bool
}

// writeDeadline bounds a single WebSocket write. TS ws sends are
// asynchronous (buffered by the socket); gorilla writes are synchronous,
// so an unbounded write could wedge the tick loop on a stuck client.
// This is an operational choice, not TS behavior: on expiry Send fails
// and Session.Flush closes the connection with code 1011, matching the
// TS send-failure close.
const writeDeadline = 10 * time.Second

// closeDeadline bounds the close handshake.
const closeDeadline = 5 * time.Second

// WebSocketChannel wraps a gorilla WebSocket connection, porting
// WebSocketBinaryChannel (BinaryChannel.ts:23-73). No compression is
// negotiated (the upgrader sets EnableCompression=false, matching
// perMessageDeflate:false in NetworkBuilder.initialize), and the read
// limit is 64 KiB (MAX_GAME_MESSAGE_BYTES, matching maxPayload).
type WebSocketChannel struct {
	conn       *websocket.Conn
	remoteAddr string

	writeMu sync.Mutex
	mu      sync.Mutex
	closed  bool
}

// NewWebSocketChannel wraps conn. It sets the 64 KiB read limit and
// resolves the remote address from req (see resolveRemoteAddr). req may
// be nil, in which case the peer address comes from the connection.
func NewWebSocketChannel(conn *websocket.Conn, req *http.Request) *WebSocketChannel {
	conn.SetReadLimit(config.MaxFrameBytes)
	return &WebSocketChannel{
		conn:       conn,
		remoteAddr: resolveRemoteAddr(conn, req),
	}
}

// Kind returns "websocket".
func (w *WebSocketChannel) Kind() string { return "websocket" }

// RemoteAddr returns the peer identity resolved at construction.
func (w *WebSocketChannel) RemoteAddr() string { return w.remoteAddr }

// Send writes payload as one binary WebSocket message. Writes are
// serialized (gorilla requires a single concurrent writer).
func (w *WebSocketChannel) Send(payload []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	w.mu.Lock()
	closed := w.closed
	w.mu.Unlock()
	if closed {
		return errors.New("session: send on closed websocket channel")
	}
	if err := w.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return err
	}
	if err := w.conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		w.markClosed()
		return err
	}
	return nil
}

// Read blocks until the next WebSocket message arrives and returns its
// payload. Like the TS ws "message" handler (BinaryChannel.ts:55-59),
// which converts any message data to a Buffer, the message type is not
// distinguished: the client speaks binary. On error the channel is
// marked closed. Only one goroutine may call Read at a time.
func (w *WebSocketChannel) Read() ([]byte, error) {
	_, payload, err := w.conn.ReadMessage()
	if err != nil {
		w.markClosed()
		return nil, err
	}
	return payload, nil
}

// Close sends a close frame with code and reason, then closes the
// underlying connection. It is idempotent.
func (w *WebSocketChannel) Close(code int, reason string) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()
	_ = w.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		time.Now().Add(closeDeadline),
	)
	_ = w.conn.Close()
}

// IsOpen reports whether the channel has not been closed or failed.
func (w *WebSocketChannel) IsOpen() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.closed
}

func (w *WebSocketChannel) markClosed() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
}

// resolveRemoteAddr ports the WebSocketBinaryChannel remoteAddress getter
// (BinaryChannel.ts:29-36): the peer socket address, except that the
// x-real-ip header is honored ONLY when the direct peer is a loopback
// proxy ("nginx overwrites this header; only a local proxy may supply
// player identity") and the header value is a valid IP. The returned
// address never carries a port: Node's socket.remoteAddress is a bare IP,
// while Go's RemoteAddr includes one, so the port is stripped.
func resolveRemoteAddr(conn *websocket.Conn, req *http.Request) string {
	peer := ""
	if req != nil {
		peer = hostOnly(req.RemoteAddr)
	}
	if peer == "" && conn != nil && conn.RemoteAddr() != nil {
		peer = hostOnly(conn.RemoteAddr().String())
	}
	if req == nil {
		return peer
	}
	switch peer {
	case "127.0.0.1", "::1", "::ffff:127.0.0.1":
		// Loopback proxy: the header may supply player identity.
	default:
		return peer
	}
	if realIP := req.Header.Get("X-Real-IP"); realIP != "" && net.ParseIP(realIP) != nil {
		return realIP
	}
	return peer
}

// hostOnly strips a trailing :port from addr, returning addr unchanged
// when it has no port or fails to split.
func hostOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// HeadlessChannel drops all output and reports itself open until closed,
// porting HeadlessBinaryChannel (BinaryChannel.ts:117-145). It lets the
// server drive a login without a real client (connectHeadlessClient in
// TS) and is the channel tests use.
//
// TS hardcodes remoteAddress "headless". NewHeadlessChannel takes an
// address so tests and bots can identify the peer; an empty address
// keeps the TS literal "headless".
type HeadlessChannel struct {
	mu   sync.Mutex
	addr string
	open bool
}

// NewHeadlessChannel returns an open headless channel for addr.
func NewHeadlessChannel(addr string) *HeadlessChannel {
	if addr == "" {
		addr = "headless"
	}
	return &HeadlessChannel{addr: addr, open: true}
}

// Kind returns "headless".
func (h *HeadlessChannel) Kind() string { return "headless" }

// RemoteAddr returns the address given at construction.
func (h *HeadlessChannel) RemoteAddr() string { return h.addr }

// Send drops payload and always succeeds, matching the TS no-op.
func (h *HeadlessChannel) Send(payload []byte) error { return nil }

// Close marks the channel closed. It is idempotent.
func (h *HeadlessChannel) Close(code int, reason string) {
	h.mu.Lock()
	h.open = false
	h.mu.Unlock()
}

// IsOpen reports whether Close has not been called.
func (h *HeadlessChannel) IsOpen() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.open
}
