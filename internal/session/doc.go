// Package session ports the TSPS connection/session layer: the transport
// abstraction, the per-player outbound packet queue, the login handshake,
// and the connection lifecycle (framing, dispatch, timeouts, teardown).
//
// TypeScript sources (server/src/main/typescript/elvarg, commit bfd81e3):
//   - net/NetworkBuilder.ts: NetworkBuilder.initialize, ClientConnection
//     (constructor/handle/dispatch/processTurnPackets/cleanup/timeouts),
//     login(), enterWorld(), sendPlayerBootstrap(), failLogin().
//   - net/BinaryChannel.ts: BinaryChannel, WebSocketBinaryChannel,
//     HeadlessBinaryChannel.
//   - net/PlayerSession.ts: sendClientPacket/flushPackets/flush queue core.
//   - net/NetworkConstants.ts: ports/queue/buffer limits.
//   - net/login/LoginTokens.ts: login flags (token crypto lives in persist).
//   - util/Misc.ts: name functions only.
//
// What this package deliberately does NOT contain (unported, absent rather
// than faked):
//   - Music and ground-item sending inside the per-tick flush, and
//     NPC-specific effects: Session.FlushTick ports flushClient
//     (see flush.go) including those three blocks (phase 8);
//     region replacements, object region changes, and the plugin
//     sync events inside flushClient are NOT STARTED. Visibility/local
//     lists live in the game package; the session reads them
//     through FlushSource.
//   - The player:bootstrap-complete plugin event and the plugin-driven
//     parts of the login sequence (login-extension hooks, settings/varp
//     restore, Sailing.onLogin): Connection.Bootstrap sends the packet
//     sequence itself (see bootstrap.go), but the plugin event is not
//     emitted and no plugin state is restored.
//   - Character select/create/delete packet flow and the plugin login
//     extension: those packets are accepted and dropped; the accounts
//     plugin owns the flow in TS.
//   - WebRTC, the TCP listener, the browser-host bridge, ContentApi,
//     and the /regions routes: the HTTP handler upgrades WebSocket
//     connections at any path, serves the GatewayTarget endpoints
//     when built with a gateway target (see gateway.go), and
//     answers 404 otherwise.
//   - Bot control (controlPlayer/releasePlayerControl/endPlayerControl),
//     stalled-input holding, and plugin-message handling: no plugin system
//     exists in Go yet.
//
// Dependency notes (adaptations to the sibling packages as they exist on
// disk, not as the original sketch described them):
//   - persist has no SaveStore/MemoryStore type, so this package defines
//     its own SaveStore interface (the minimal load/save port Authenticate
//     needs). The coordinator supplies an implementation; when persist
//     lands a store, the wiring adapts it to this interface.
//   - persist.VerifyAndRotate/IssueToken operate on []persist.TokenEntry,
//     and persist.PlayerSave exposes Tokens/SetTokens; login uses those.
//   - persist.Accounts.NameAvailable is documented partial in persist
//     itself, so the new-account name check uses
//     persist.DisplayNames.OwnerOf (the TS login() check) via the optional
//     LoginDeps.DisplayNames field instead.
package session
