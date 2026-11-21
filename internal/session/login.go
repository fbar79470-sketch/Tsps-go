package session

import (
	"strings"
	"time"

	"tsps-go/internal/persist"
)

// Login response codes, sent literally by the live login path
// (NetworkBuilder.ts login()/failLogin). Do NOT "fix" these to the
// LoginResponses.ts constants: that file is legacy and disagrees (its
// world-full is 7; the live path sends 2), and code 8 does not exist in
// it at all. See ANALYSIS §5.2 step 5.
const (
	// LoginCodeWorldFull is sent when the world is full (code 2).
	LoginCodeWorldFull int32 = 2
	// LoginCodeInvalid is sent for an invalid name/password, a deleted
	// name, or an unavailable name (code 3).
	LoginCodeInvalid int32 = 3
	// LoginCodeBanned is sent for a banned account (code 4).
	LoginCodeBanned int32 = 4
	// LoginCodeAlreadyOnline is sent when the account is already logged
	// in or a login is pending (code 5).
	LoginCodeAlreadyOnline int32 = 5
	// LoginCodeOutdated is sent on a revision mismatch (code 6).
	LoginCodeOutdated int32 = 6
	// LoginCodeBusy is sent when the login server cannot serve the
	// request: unreadable registries, save load failures, hashing
	// failures (code 8).
	LoginCodeBusy int32 = 8
	// LoginCodeIPBanned is sent for a banned IP address (code 27).
	LoginCodeIPBanned int32 = 27
)

// Login failure messages, verbatim from NetworkBuilder.ts login().
const (
	msgOutdated      = "Please close the client and reload to update."
	msgInvalid       = "Invalid username or password."
	msgBanned        = "Your account has been disabled."
	msgIPBanned      = "Your IP address has been banned."
	msgDeleted       = "That name belonged to a deleted character and can't be used again."
	msgWorldFull     = "This world is full."
	msgAlreadyOnline = "Your account is already logged in."
	msgBusy          = "The login server is busy."
	msgNameTaken     = "That name is not available. Please choose another."
)

// Login flags live in persist (persist.LoginFlagRemember=1,
// persist.LoginFlagToken=2, persist.LoginFlagCharacterSelect=4), ported
// from net/login/LoginTokens.ts:15-19.

// SaveStore is the minimal save-backend surface Authenticate needs: load
// a character's save for a profile, and write one back. Load returns
// (nil, nil) when no save exists; an error only on backend failure.
//
// persist.SaveStore exists but has different semantics (Load reports a
// missing save as persist.ErrSaveNotFound, and Save takes the save plus
// a reason and derives the key from the save itself), so this interface
// lives here: the session defines the port it needs and the coordinator
// supplies an adapter over a persist store. Save is
// used only for the best-effort remembered-login writes (see
// saveLoginTokens): TS persists rotated/revoked/issued token entries
// before answering, warning and continuing on failure.
type SaveStore interface {
	Load(username string, profile persist.Profile) (*persist.PlayerSave, error)
	Save(username string, profile persist.Profile, save *persist.PlayerSave) error
}

// PendingLogin is an authenticated login awaiting the handshake packet
// that enters the world. It ports PluginLoginPending as built by
// login() (NetworkBuilder.ts:1604), plus two carried fields TS keeps on
// the same object:
//
//   - LoginTokens carries remembered-login entries that have nowhere
//     else to live yet: entries issued to a brand-new account (no save
//     to persist them to) and entries of an unmade save (whose
//     characterSave is nil). TS carries them as pending.loginTokens and
//     enterWorld applies them to the player's attributes when the
//     entering character is the primary. Nil when no token operation
//     produced entries.
//   - CharacterSelect records the LOGIN_FLAG_CHARACTER_SELECT flag. The
//     character-select packet flow itself is NOT implemented (owned by
//     the accounts login-extension plugin in TS); the flag is recorded
//     so the wiring can see the client expects it.
type PendingLogin struct {
	Username       string
	CredentialName string
	DisplayName    string
	Profile        string
	PasswordHash   string
	Save           *persist.PlayerSave
	LoginToken     string

	LoginTokens     []persist.TokenEntry
	CharacterSelect bool
}

// BootstrapWorld carries the world-level facts the login bootstrap
// (Connection.Bootstrap, sendPlayerBootstrap in NetworkBuilder.ts)
// needs that no per-player state supplies: whether the world is a
// members world (isMembersWorld, WorldDefinition.ts:307 — true unless
// the FreeToPlay plugin switch is enabled), the world-type label
// script 876 shows (GameConstants.WORLD_TYPE.label, GameConstants.ts:54
// — "REGULAR" unless a game mode claims the world through setWorldType),
// and the configured developer usernames (DEV_USERNAME, granted
// DEVELOPER rights on login by enterWorld, NetworkBuilder.ts:1670).
//
// A nil *BootstrapWorld on LoginDeps means the TS defaults: a members
// world, label "REGULAR", no developer usernames. An empty TypeLabel
// likewise means "REGULAR".
type BootstrapWorld struct {
	MembersWorld bool
	TypeLabel    string
	DevUsernames []string
}

// ItemWeightSource answers the effective carried weight, in grams,
// of one occupied inventory or equipment slot holding the given item
// id: the item definition's weight, counted once per slot regardless
// of the stack amount, or 0 for a missing id and for stackable or
// noted items. It is the definition half of Player.getCarriedWeightGrams
// (Player.ts:1287-1311); the slot iteration lives in
// carriedWeightKg (bootstrap.go). Implementations must be total:
// every id, including negative and out-of-range ids, returns a
// weight (0 when the definition is unknown) and never fails.
//
// The production implementation reads CacheDefinitions (cmd/server);
// a nil ItemWeightSource on LoginDeps means no definitions are
// loaded and the bootstrap sends weight 0.
type ItemWeightSource interface {
	ItemWeightGrams(id int) int
}

// LoginDeps supplies everything Authenticate needs from the world
// around it. The coordinator builds one per server; Authenticate never
// touches global state.
//
// BootstrapWorld and ItemWeights are read by the connection's
// bootstrap, not by Authenticate; they live here because LoginDeps is
// the connection's deps bag and the wiring builds exactly one per
// server.
type LoginDeps struct {
	// ActiveRevision is the cache revision clients must report
	// (CachePipeline.getActive().revision in TS).
	ActiveRevision int

	// Accounts resolves typed names to accounts (nil fails every login
	// with code 8, matching TS behavior when accounts.json is
	// unreadable).
	Accounts *persist.Accounts
	// Punishment answers ban/IP-ban checks (nil means no bans).
	Punishment *persist.Punishment
	// DisplayNames answers the new-account display-name check (nil
	// skips that sub-check; see Authenticate step 8). Optional.
	DisplayNames *persist.DisplayNames
	// Profile is the world profile saves load under
	// (GameConstants.WORLD_PROFILE in TS). "" means ProfileMain.
	// Optional; league/hardcore worlds set it.
	Profile persist.Profile
	// Saves loads and writes character saves (nil behaves like TS
	// NullPlayerPersistence: no saves exist, writes are no-ops).
	Saves SaveStore

	// IsOnline reports whether any character of an account is online
	// or queued to enter the world. It receives pendingKey-form keys
	// (lowercase, underscores: FormatNameForProtocol of the name), one
	// per character name, matching the TS checks against
	// World.getPlayerByName (protocol-form comparison, World.ts:135)
	// and the add-player queue (NetworkBuilder.ts:1458). The wiring
	// MUST include queued players: TS refuses a login when a character
	// is in the add queue. Nil means offline.
	IsOnline func(accountKey string) bool
	// ReserveName atomically reserves a pending-login name key
	// (pendingKey form), returning false when another login already
	// holds it. It backs the static pendingNames set in TS
	// (NetworkBuilder.ts:442, reservePendingName). Nil means no-op
	// allow (single-connection tests); production wiring MUST supply
	// a shared reservation set.
	ReserveName func(key string) bool
	// ReleaseName releases a key reserved through ReserveName. Nil
	// means no-op.
	ReleaseName func(key string)
	// WorldFull reports whether the world player list is full. Nil
	// means not full.
	WorldFull func() bool
	// BootstrapWorld carries the world-level facts the login bootstrap
	// needs (see BootstrapWorld). Nil means the TS defaults: a members
	// world, label "REGULAR", no developer usernames.
	BootstrapWorld *BootstrapWorld
	// ItemWeights supplies item-definition weights for the login
	// bootstrap's run-energy packet (see ItemWeightSource). Nil
	// means no definitions are loaded (NoCache mode): the bootstrap
	// sends weight 0, which is exact for empty containers.
	ItemWeights ItemWeightSource
	// Now is the clock (nil means time.Now).
	Now func() time.Time
}

func (d *LoginDeps) now() time.Time {
	if d != nil && d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *LoginDeps) profile() persist.Profile {
	if d != nil && d.Profile != "" {
		return d.Profile
	}
	return persist.ProfileMain
}

// Authenticate ports NetworkBuilder.login() (NetworkBuilder.ts:1394-1627):
// the check order and literal response codes, exactly. It returns a
// non-nil PendingLogin on success (code 0, empty message) or
// (nil, code, message) on failure, where code/message are what
// failLogin sends and the socket stays open for retry.
//
// Check order (failures release any name reservation before returning):
//  1. revision mismatch -> 6
//  2. protocol name length 1-12, valid name, password length 1-64 -> 3
//  3. account banned -> 4; IP banned -> 27
//  4. resolve account; deleted-name tombstone -> 3; banned primary -> 4
//  5. world full -> 2; any character online/queued/reserved -> 5; then
//     reserve the account name
//  6. load saves; load failure -> 8
//  7. new account: name shown/held by another account -> 3
//  8. token login without a save -> 3
//  9. token verification (rotation; reuse revokes) or password check
//     (scrypt via persist.CheckPassword, legacy plaintext fallback
//     inside persist; new accounts hash immediately); wrong -> 3;
//     hashing/token failures -> 8
//  10. remember flag issues a token (persisted when a save exists)
//  11. build the pending login (alt-character save load; unmade saves
//     enter as new characters); reserve the character name
//
// Differences from TS, all forced by the Go shape and documented:
//   - The plugin login extension hooks (authenticated/accepted) do not
//     exist: no renaming, no redirect, no character-select interception.
//     The CHARACTER_SELECT flag is recorded on PendingLogin only.
//   - The shownByAnother live-player check in step 7 is NOT implemented:
//     it needs online players' display names, which no LoginDeps field
//     carries (IsOnline is account-key based). The DisplayNames
//     owner/hold check is implemented when DisplayNames is supplied.
//   - TS re-checks this.closed after its async password/token work; Go
//     Authenticate runs synchronously on the connection's read loop, so
//     no interleaved close is possible and no such check exists.
func Authenticate(deps *LoginDeps, username, password string, revision int32, flags byte, remoteAddr string) (*PendingLogin, int32, string) {
	if deps == nil {
		return nil, LoginCodeBusy, msgBusy
	}
	now := deps.now()
	profile := deps.profile()
	tokenLogin := flags&persist.LoginFlagToken != 0

	typedName := FormatDisplayName(username)
	protocolName := FormatNameForProtocol(typedName)

	// 1. Revision.
	if int(revision) != deps.ActiveRevision {
		return nil, LoginCodeOutdated, msgOutdated
	}
	// 2. Name/password shape. Lengths count UTF-16 code units, as JS
	// string.length does.
	if utf16Len(protocolName) < 1 || utf16Len(protocolName) > 12 ||
		!IsValidName(protocolName) ||
		utf16Len(password) < 1 || utf16Len(password) > 64 {
		return nil, LoginCodeInvalid, msgInvalid
	}
	// 3. Bans.
	if deps.Punishment != nil {
		if deps.Punishment.Banned(typedName) {
			return nil, LoginCodeBanned, msgBanned
		}
		if deps.Punishment.IPBanned(remoteAddr) {
			return nil, LoginCodeIPBanned, msgIPBanned
		}
	}
	// 4. Account resolution.
	if deps.Accounts == nil {
		return nil, LoginCodeBusy, msgBusy
	}
	accountKey, found := deps.Accounts.AccountOf(typedName)
	deletedName := !found && deps.Accounts.IsDeleted(typedName)
	primaryName := typedName
	if found {
		if primary, ok := deps.Accounts.PrimaryOf(accountKey); ok {
			primaryName = primary
		}
	}
	accountNames := []string{typedName}
	if found {
		for _, ch := range deps.Accounts.CharactersOf(accountKey) {
			accountNames = append(accountNames, ch.Name)
		}
	}
	if deletedName {
		return nil, LoginCodeInvalid, msgDeleted
	}
	if deps.Punishment != nil && deps.Punishment.Banned(primaryName) {
		// A banned primary bans the whole account, whichever of its
		// characters' names was typed.
		return nil, LoginCodeBanned, msgBanned
	}
	// 5. World full / already online / pending. TS checks all three in
	// one condition with world-full taking the code; the pending check
	// is a test-and-set reservation here (see LoginDeps.ReserveName).
	// Checking only the account key is equivalent to TS's
	// accountKeys.some(pendingNames.has): every in-flight login holds
	// its account key from this step until release, so a held character
	// key always implies a held account key for the same account.
	full := deps.WorldFull != nil && deps.WorldFull()
	online := false
	if deps.IsOnline != nil {
		for _, name := range accountNames {
			if deps.IsOnline(pendingKey(name)) {
				online = true
				break
			}
		}
	}
	if full || online {
		if full {
			return nil, LoginCodeWorldFull, msgWorldFull
		}
		return nil, LoginCodeAlreadyOnline, msgAlreadyOnline
	}
	accountPendingKey := pendingKey(typedName)
	if found {
		accountPendingKey = pendingKey(accountKey)
	}
	reserved := false
	release := func() {
		if reserved && deps.ReleaseName != nil {
			deps.ReleaseName(accountPendingKey)
			reserved = false
		}
	}
	if deps.ReserveName != nil {
		if !deps.ReserveName(accountPendingKey) {
			return nil, LoginCodeAlreadyOnline, msgAlreadyOnline
		}
		reserved = true
	}
	fail := func(code int32, msg string) (*PendingLogin, int32, string) {
		release()
		return nil, code, msg
	}

	// 6. Load saves. The credential is the main character's save; the
	// profile save and then any other profile's save only stand in
	// while no main character exists (NetworkBuilder.ts:1476-1488).
	load := func(name string, prof persist.Profile) (*persist.PlayerSave, error) {
		if deps.Saves == nil {
			return nil, nil
		}
		return deps.Saves.Load(name, prof)
	}
	save, err := load(primaryName, profile)
	if err != nil {
		return fail(LoginCodeBusy, msgBusy)
	}
	var mainSave *persist.PlayerSave
	if profile == persist.ProfileMain {
		mainSave = save
	} else {
		mainSave, err = load(primaryName, persist.ProfileMain)
		if err != nil {
			return fail(LoginCodeBusy, msgBusy)
		}
	}
	credentials := mainSave
	if credentials == nil {
		credentials = save
	}
	for _, other := range persist.OtherProfiles(profile) {
		if credentials != nil {
			break
		}
		if other == persist.ProfileMain {
			continue
		}
		credentials, err = load(primaryName, other)
		if err != nil {
			return fail(LoginCodeBusy, msgBusy)
		}
	}

	// 7. A new account can't take a name another account shows or still
	// holds (DisplayNames.ownerOf). The live-player shownByAnother
	// check is not implemented; see the doc comment above.
	if credentials == nil && deps.DisplayNames != nil {
		if owner, owned := deps.DisplayNames.OwnerOf(typedName, now); owned && owner != persist.Key(typedName) {
			return fail(LoginCodeInvalid, msgNameTaken)
		}
	}

	// 8. A remembered login only signs in to an account whose primary
	// exists (its tokens live in that save).
	if tokenLogin && save == nil {
		return fail(LoginCodeInvalid, msgInvalid)
	}

	// 9. Verify the credential.
	var passwordHash, loginToken string
	var loginTokens []persist.TokenEntry
	saveLoginTokens := func(entries []persist.TokenEntry) {
		// Best-effort, as TS saveLoginTokens (NetworkBuilder.ts:1916):
		// the entries are written before answering so a swapped token
		// is on disk even if the player never finishes logging in; a
		// write failure only warns (here: ignored) and login continues.
		if save == nil || deps.Saves == nil {
			return
		}
		if err := save.SetTokens(entries); err != nil {
			return
		}
		_ = deps.Saves.Save(primaryName, profile, save)
	}
	if tokenLogin {
		entries := save.Tokens(now)
		updated, rotated, ok := persist.VerifyAndRotate(entries, password, now)
		if !ok {
			// A reused (rotated-out) token revokes that browser's
			// entry: VerifyAndRotate drops it from the list, so a
			// shorter list on failure means a revocation happened
			// and must be persisted. A plain invalid token persists
			// nothing (TS only writes on the reused branch).
			if len(updated) < len(entries) {
				saveLoginTokens(updated)
			}
			return fail(LoginCodeInvalid, msgInvalid)
		}
		loginToken = rotated
		loginTokens = updated
		saveLoginTokens(updated)
		passwordHash = save.PasswordHashWithSalt
	} else if credentials != nil {
		stored := credentials.PasswordHashWithSalt
		valid := persist.CheckPassword(password, stored)
		if stored != "" && !valid {
			return fail(LoginCodeInvalid, msgInvalid)
		}
		// An empty stored hash means "no credential yet": the account
		// adopts the typed password (TS: stored && !valid is false,
		// then encryptPassword). A legacy plaintext match is upgraded
		// to a hash the same way; only a scrypt match keeps the
		// stored value.
		if valid && strings.Contains(stored, ":") {
			passwordHash = stored
		} else {
			passwordHash, err = persist.HashPassword(password)
			if err != nil {
				return fail(LoginCodeBusy, msgBusy)
			}
		}
	} else {
		// New account: hash the supplied password immediately.
		passwordHash, err = persist.HashPassword(password)
		if err != nil {
			return fail(LoginCodeBusy, msgBusy)
		}
	}

	// 10. Remember flag: issue a token (persisted when a save exists;
	// otherwise carried on the pending login, see PendingLogin).
	if !tokenLogin && flags&persist.LoginFlagRemember != 0 {
		var entries []persist.TokenEntry
		if save != nil {
			entries = save.Tokens(now)
		}
		updated, issued, err := persist.IssueToken(entries, now)
		if err != nil {
			return fail(LoginCodeBusy, msgBusy)
		}
		loginToken = issued
		loginTokens = updated
		if save != nil {
			saveLoginTokens(updated)
		}
	}
	// An unmade save vouched for the login above; its character is a
	// new one, so its remembered logins are carried across here or the
	// first save would drop them (NetworkBuilder.ts:1570-1573).
	if loginTokens == nil && save != nil && save.Unmade {
		loginTokens = save.Tokens(now)
	}

	// 11. Build the pending login.
	usernameOut := primaryName
	if found && persist.Key(typedName) != persist.Key(primaryName) {
		// No login extension in Go: an alt logs in under its typed
		// name (NetworkBuilder.ts:1587).
		usernameOut = typedName
	}
	characterSave := save
	if characterSave != nil && characterSave.Unmade {
		characterSave = nil
	}
	if usernameOut != primaryName {
		characterSave, err = load(usernameOut, profile)
		if err != nil {
			return fail(LoginCodeBusy, msgBusy)
		}
		if characterSave != nil && characterSave.Unmade {
			characterSave = nil
		}
		// The alt enters with the capitals its save was made with.
		if characterSave != nil && characterSave.DisplayName != "" &&
			persist.Key(characterSave.DisplayName) == persist.Key(usernameOut) {
			usernameOut = characterSave.DisplayName
		}
	}
	pending := &PendingLogin{
		Username:        usernameOut,
		CredentialName:  primaryName,
		Profile:         string(profile),
		PasswordHash:    passwordHash,
		Save:            characterSave,
		LoginToken:      loginToken,
		LoginTokens:     loginTokens,
		CharacterSelect: flags&persist.LoginFlagCharacterSelect != 0,
	}
	// The response name: the capitals the primary's save was made
	// with, when they key to the primary (NetworkBuilder.ts:1605-1608).
	var savedName string
	if save != nil {
		savedName = save.DisplayName
	} else if credentials != nil {
		savedName = credentials.DisplayName
	}
	responseName := primaryName
	if savedName != "" && persist.Key(savedName) == persist.Key(primaryName) {
		responseName = savedName
	}
	if persist.Key(pending.Username) == persist.Key(primaryName) {
		pending.Username = responseName
	}
	pending.DisplayName = responseName

	// Reserve the character name (TS reserves it after the extension
	// hook; there is no hook here). Failure means a racing login holds
	// it: refuse as already-online, releasing the account key.
	characterKey := pendingKey(pending.Username)
	if deps.ReserveName != nil && characterKey != accountPendingKey {
		if !deps.ReserveName(characterKey) {
			return fail(LoginCodeAlreadyOnline, msgAlreadyOnline)
		}
		// The character reservation is released together with the
		// account reservation by the connection, which recomputes
		// both keys from the pending login; track it here only so a
		// later failure in this function would release it too. No
		// failure path remains after this point.
	}
	return pending, 0, ""
}
