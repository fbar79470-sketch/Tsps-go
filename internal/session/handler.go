package session

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// NewHandler returns the HTTP handler for the game server, porting the
// NetworkBuilder.initialize equivalent (NetworkBuilder.ts:347-434) in
// minimal form: one HTTP server, WebSocket upgrade at any path, the
// gateway endpoints when gateway is non-nil, and 404 for everything
// else. A nil gateway preserves the old behaviour: every non-WebSocket
// request gets 404. The coordinator wires it in cmd/server with
// http.ListenAndServe on the configured WebSocket port.
//
// Gateway routing mirrors TS (NetworkBuilder.ts:347-371): no method
// check on gateway paths, and Access-Control-Allow-Origin: * on every
// response. The match is against the raw (escaped) path —
// r.URL.EscapedPath(), not the decoded r.URL.Path — because TS
// matches the raw request URL string; a %XX escape in the path must
// not decode into a gateway path.
//
//   - The WebSocket server attaches to the HTTP server with no path
//     restriction in TS (new WebSocketServer({server: http, ...})), so
//     any path upgrades. The upgrader disables compression
//     (perMessageDeflate: false) and the channel enforces the 64 KiB
//     read limit (maxPayload: MAX_GAME_MESSAGE_BYTES).
//   - Non-WebSocket HTTP requests get 404, except the gateway
//     endpoints (GatewayTarget: /gateway/jav_config.ws and
//     /gateway/worldlist.ws), served when the handler was built
//     with a non-nil gateway target. The TS routes served on the
//     same port that are NOT implemented in this run: ContentApi (/api/*),
//     the /regions endpoints, and the browser-host bridge page/handler
//     (BROWSER_HOST=1). WebRTC registration is not implemented either.
//   - Like TS, cross-origin WebSocket handshakes are accepted (the ws
//     library performs no origin check by default; the TS HTTP handler
//     also sets Access-Control-Allow-Origin: *).
//
// newConn builds the Connection for each upgraded channel; it is called
// synchronously during the upgrade. The handler sends the welcome
// packet, starts the idle/pending-login checks, and runs the read loop
// until the connection closes or errors, then cleans up.
func NewHandler(newConn func(Channel) *Connection, gateway *GatewayTarget) http.Handler {
	upgrader := websocket.Upgrader{
		EnableCompression: false,
		CheckOrigin:       func(r *http.Request) bool { return true },
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isWebSocketUpgrade(r) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			if gateway != nil {
				if contentType, body, ok := gateway.Resolve(r.URL.EscapedPath()); ok {
					w.Header().Set("Content-Type", contentType)
					_, _ = w.Write(body)
					return
				}
			}
			http.NotFound(w, r)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Upgrade has written the error response.
			return
		}
		ch := NewWebSocketChannel(conn, r)
		c := newConn(ch)
		if c == nil {
			ch.Close(1011, "connection rejected")
			return
		}
		// Ensure teardown on every exit: release world/pending state,
		// then close the transport (idempotent; a no-op when the
		// connection already closed it with a specific code).
		defer func() {
			c.Cleanup()
			ch.Close(1000, "")
		}()
		if err := c.SendWelcome(int32(time.Now().UnixMilli())); err != nil {
			return
		}
		stop := make(chan struct{})
		defer close(stop)
		c.StartIdleCheck(stop)
		for {
			frame, err := ch.Read()
			if err != nil {
				return
			}
			if err := c.HandleFrame(frame); err != nil {
				var closeErr *CloseError
				if errors.As(err, &closeErr) {
					c.Close(closeErr.Code, closeErr.Reason)
				} else {
					c.Close(1011, "internal error")
				}
				return
			}
			if c.IsClosed() {
				// Dispatch closed the connection itself (e.g. the
				// turn-packet cap); stop reading.
				return
			}
		}
	})
}

// isWebSocketUpgrade reports whether r is a WebSocket handshake: an
// Upgrade header of "websocket" and a Connection header containing the
// "upgrade" token. Anything else is a plain HTTP request and gets a 404.
func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, token := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			return true
		}
	}
	return false
}
