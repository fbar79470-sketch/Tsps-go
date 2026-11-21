// GatewayTarget ports net/http/GatewayTarget.ts (bfd81e3): the two
// static endpoints the RSProx gateway process reads from the game
// server to point the official client at this world —
// GET /gateway/jav_config.ws and GET /gateway/worldlist.ws — served
// on the game port by the same HTTP listener as the WebSocket
// (NetworkBuilder.ts:347-434).
//
// The resolver is pure apart from construction: it holds the
// resolved host, port, revision, members flag, and world-type
// bits/activity, and never reads the environment at request time.
// PUBLIC_HOST is read once, at construction.
package session

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
)

// JavConfigPath and WorldListPath are GatewayTarget.JAV_CONFIG_PATH
// and WORLD_LIST_PATH (GatewayTarget.ts).
const (
	JavConfigPath = "/gateway/jav_config.ws"
	WorldListPath = "/gateway/worldlist.ws"
)

// defaultGatewayHost is GatewayTarget.DEFAULT_HOST.
const defaultGatewayHost = "127.0.0.1"

// gatewayWorldID is GatewayTarget.WORLD_ID.
const gatewayWorldID = 1

// gatewayFlagMembers is GatewayTarget.WORLD_FLAG_MEMBERS.
const gatewayFlagMembers = 0x1

// bareHostPattern is GatewayTarget.HOST_PATTERN: a bare host name
// or address — no scheme, port, path, or spaces.
var bareHostPattern = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// validBareHost reports whether host matches the bare-host pattern
// (GatewayTarget.HOST_PATTERN). It is the pure validation half of
// GatewayTarget.host, kept separate so tests can drive it without
// the environment.
func validBareHost(host string) bool {
	return bareHostPattern.MatchString(host)
}

// GatewayTarget is the resolved gateway endpoint set for one world:
// the host the jav_config and world list advertise, the game port
// the worldlist URL names, the active cache revision, the members
// flag, and the world type's flags/activity.
//
// Activity "" means the TS default: GameConstants.WORLD_TYPE is
// { label: "REGULAR", flags: 0, activity: null } (GameConstants.ts:54),
// so a world with no type override advertises activity "tsps" and
// type flags 0. The Go world definition carries no world-type
// source (no setWorldType equivalent; data/definitions/world.json
// has no type/flags/activity key), so buildServer passes flags 0
// and activity "" — the TS-faithful behaviour for this world, not
// an invented type field.
//
// warned records the invalid PUBLIC_HOST values already warned
// about, so each distinct bad value is reported exactly once
// (GatewayTarget.warnedHost in TS, which is a single static slot;
// here a per-target set, guarded by mu).
type GatewayTarget struct {
	host         string
	port         int
	revision     int
	membersWorld bool
	typeFlags    uint32
	activity     string
	logger       *log.Logger

	mu     sync.Mutex
	warned map[string]bool
}

// NewGatewayTarget resolves the gateway endpoints for one world.
// It reads PUBLIC_HOST once, at construction: trimmed, and used
// only when it is a bare host name or IP address; anything else is
// warned about once per distinct value through logger (a nil
// logger is silent) and ignored, falling back to 127.0.0.1
// (GatewayTarget.host). Port is the game port the worldlist URL
// names; revision is the active cache revision (param=25);
// typeFlags/activity are the world type's bits and activity text,
// with "" activity meaning the TS default "tsps".
func NewGatewayTarget(port, revision int, membersWorld bool, typeFlags uint32, activity string, logger *log.Logger) *GatewayTarget {
	g := &GatewayTarget{
		port:         port,
		revision:     revision,
		membersWorld: membersWorld,
		typeFlags:    typeFlags,
		activity:     activity,
		logger:       logger,
		warned:       map[string]bool{},
	}
	g.host = g.resolveHost(os.Getenv("PUBLIC_HOST"))
	return g
}

// resolveHost is GatewayTarget.host: trim the raw PUBLIC_HOST
// value, return the default when it is empty, return it when it
// is a bare host, and otherwise warn once per distinct value and
// return the default. It is a method (rather than a package
// function) so the warn-once set lives on the target.
func (g *GatewayTarget) resolveHost(raw string) string {
	configured := strings.TrimSpace(raw)
	if configured == "" {
		return defaultGatewayHost
	}
	if validBareHost(configured) {
		return configured
	}
	g.mu.Lock()
	alreadyWarned := g.warned[configured]
	if !alreadyWarned {
		g.warned[configured] = true
	}
	g.mu.Unlock()
	if !alreadyWarned && g.logger != nil {
		g.logger.Printf(`[gateway] PUBLIC_HOST %q isn't a host name or IP address; using %s`, configured, defaultGatewayHost)
	}
	return defaultGatewayHost
}

// Host returns the host the jav_config and world list advertise.
func (g *GatewayTarget) Host() string { return g.host }

// JavConfig returns the jav_config body (GatewayTarget.javConfig),
// the official client's jav_config keys with the codebase and
// world list pointed here and param=25 naming the active cache's
// revision. The lines are transcribed verbatim from
// GatewayTarget.ts, joined with "\n", so the body ends with a
// newline (the trailing empty line). Live 242 params 30/31 are
// NOT sent (TS comment). The body is latin1, and every byte here
// is ASCII, so the string converts to bytes unchanged.
func (g *GatewayTarget) JavConfig() string {
	worldList := fmt.Sprintf("http://%s:%d%s", g.host, g.port, WorldListPath)
	return strings.Join([]string{
		"title=tsps",
		fmt.Sprintf("codebase=http://%s/", g.host),
		"cachedir=tsps",
		"storebase=0",
		"initial_jar=gamepack.jar",
		"initial_class=client.class",
		"viewerversion=124",
		"win_sub_version=1",
		"mac_sub_version=2",
		"other_sub_version=2",
		"window_preferredwidth=800",
		"window_preferredheight=600",
		"advert_height=96",
		"applet_minwidth=765",
		"applet_minheight=503",
		"applet_maxwidth=5760",
		"applet_maxheight=2160",
		"param=2=",
		"param=3=true",
		"param=4=1",
		"param=5=1",
		"param=7=0",
		"param=8=true",
		"param=9=",
		"param=10=5",
		"param=11=",
		fmt.Sprintf("param=12=%d", gatewayWorldID),
		"param=13=.runescape.com",
		"param=14=0",
		"param=15=0",
		"param=16=false",
		fmt.Sprintf("param=17=%s", worldList),
		"param=18=",
		"param=19=",
		"param=20=",
		"param=21=0",
		"param=22=",
		fmt.Sprintf("param=25=%d", g.revision),
		"param=28=",
		"param=29=1",
		"",
	}, "\n")
}

// WorldList returns the one-world binary list
// (GatewayTarget.worldList): u32 length, u16 count, then per world
// u16 id, u32 flags, host NUL, activity NUL, u8 location, u16
// population. Flags are the members bit (0x1 when this is a
// members world) plus the world type's own bits; activity is the
// world type's, else "tsps"; location and population are 0. The
// length field is 2 + the world-record length, as in TS.
func (g *GatewayTarget) WorldList() []byte {
	activity := g.activity
	if activity == "" {
		activity = "tsps"
	}
	flags := uint32(0)
	if g.membersWorld {
		flags |= gatewayFlagMembers
	}
	flags |= g.typeFlags

	world := make([]byte, 0, 64)
	var two [2]byte
	binary.BigEndian.PutUint16(two[:], gatewayWorldID)
	world = append(world, two[:]...)
	var four [4]byte
	binary.BigEndian.PutUint32(four[:], flags)
	world = append(world, four[:]...)
	world = append(world, []byte(g.host)...)
	world = append(world, 0)
	world = append(world, []byte(activity)...)
	world = append(world, 0)
	world = append(world, 0)
	world = append(world, 0, 0)

	out := make([]byte, 0, 6+len(world))
	binary.BigEndian.PutUint32(four[:], uint32(2+len(world)))
	out = append(out, four[:]...)
	binary.BigEndian.PutUint16(two[:], 1)
	out = append(out, two[:]...)
	return append(out, world...)
}

// Resolve returns the gateway response for url, or ok=false when
// it is not a gateway path (GatewayTarget.resolve). The URL is
// split on "?" and only the two exact paths match, as in TS.
func (g *GatewayTarget) Resolve(url string) (contentType string, body []byte, ok bool) {
	path, _, _ := strings.Cut(url, "?")
	switch path {
	case JavConfigPath:
		return "text/plain; charset=iso-8859-1", []byte(g.JavConfig()), true
	case WorldListPath:
		return "application/octet-stream", g.WorldList(), true
	default:
		return "", nil, false
	}
}
