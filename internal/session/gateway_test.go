package session

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"testing"
)

// expectedJavConfig builds the jav_config body the TS source
// produces for the given host, port, and revision, from the same
// 30 TS lines (GatewayTarget.javConfig, GatewayTarget.ts) —
// written out here independently of GatewayTarget.JavConfig so
// the test is a byte-exact check against the TS text, not a
// restatement of the implementation.
func expectedJavConfig(host string, port, revision int) string {
	lines := []string{
		"title=tsps",
		"codebase=http://" + host + "/",
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
		"param=12=1",
		"param=13=.runescape.com",
		"param=14=0",
		"param=15=0",
		"param=16=false",
		"param=17=http://" + host + fmt.Sprintf(":%d", port) + "/gateway/worldlist.ws",
		"param=18=",
		"param=19=",
		"param=20=",
		"param=21=0",
		"param=22=",
		fmt.Sprintf("param=25=%d", revision),
		"param=28=",
		"param=29=1",
		"",
	}
	return strings.Join(lines, "\n")
}

func TestGatewayJavConfigDefault(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "")
	g := NewGatewayTarget(43594, 242, true, 0, "", nil)
	if got := g.JavConfig(); got != expectedJavConfig("127.0.0.1", 43594, 242) {
		t.Errorf("JavConfig default mismatch:\n got: %q\nwant: %q", got, expectedJavConfig("127.0.0.1", 43594, 242))
	}
	if got := g.Host(); got != "127.0.0.1" {
		t.Errorf("Host() = %q, want 127.0.0.1", got)
	}
}

func TestGatewayJavConfigPublicHost(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "  play.example.com  ")
	g := NewGatewayTarget(43594, 242, true, 0, "", nil)
	if got := g.JavConfig(); got != expectedJavConfig("play.example.com", 43594, 242) {
		t.Errorf("JavConfig PUBLIC_HOST mismatch:\n got: %q\nwant: %q", got, expectedJavConfig("play.example.com", 43594, 242))
	}
}

func TestGatewayJavConfigCustomPortAndRevision(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "")
	g := NewGatewayTarget(49700, 241, false, 0, "", nil)
	if got := g.JavConfig(); got != expectedJavConfig("127.0.0.1", 49700, 241) {
		t.Errorf("JavConfig port/revision mismatch:\n got: %q\nwant: %q", got, expectedJavConfig("127.0.0.1", 49700, 241))
	}
	// The worldlist URL names the actual port (param=17), and the
	// revision is the active cache's (param=25).
	if !strings.Contains(g.JavConfig(), "param=17=http://127.0.0.1:49700/gateway/worldlist.ws") {
		t.Errorf("JavConfig missing port-49700 worldlist URL: %q", g.JavConfig())
	}
	if !strings.Contains(g.JavConfig(), "param=25=241") {
		t.Errorf("JavConfig missing param=25=241: %q", g.JavConfig())
	}
}

func TestGatewayJavConfigShape(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "")
	g := NewGatewayTarget(43594, 242, true, 0, "", nil)
	body := g.JavConfig()
	// 41 joined entries (40 content lines + trailing empty line):
	// the body ends with a newline, and there are no live-242
	// params 30/31.
	if !strings.HasSuffix(body, "\n") {
		t.Errorf("JavConfig does not end with a newline: %q", body)
	}
	if parts := strings.Split(body, "\n"); len(parts) != 41 || parts[len(parts)-1] != "" {
		t.Errorf("JavConfig = %d newline-split parts, want 41 with a trailing empty part", len(parts))
	}
	for _, param := range []string{"param=30=", "param=31="} {
		if strings.Contains(body, param) {
			t.Errorf("JavConfig contains %s, want live-242 params 30/31 absent", param)
		}
	}
	// latin1: every byte is ASCII.
	for i := 0; i < len(body); i++ {
		if body[i] > 0x7f {
			t.Errorf("JavConfig byte %d = %#x, want ASCII only", i, body[i])
		}
	}
}

func TestGatewayInvalidHostIgnoredAndWarnedOnce(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "https://example.com/")
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	g := NewGatewayTarget(43594, 242, true, 0, "", logger)
	if got := g.Host(); got != "127.0.0.1" {
		t.Errorf("Host() with invalid PUBLIC_HOST = %q, want 127.0.0.1", got)
	}
	if got := g.JavConfig(); got != expectedJavConfig("127.0.0.1", 43594, 242) {
		t.Errorf("JavConfig with invalid PUBLIC_HOST mismatch:\n got: %q\nwant: %q", got, expectedJavConfig("127.0.0.1", 43594, 242))
	}
	if warnings := strings.Count(buf.String(), "[gateway]"); warnings != 1 {
		t.Errorf("invalid host warned %d times, want exactly 1: %q", warnings, buf.String())
	}
	if !strings.Contains(buf.String(), `PUBLIC_HOST "https://example.com/"`) {
		t.Errorf("warning does not name the invalid value: %q", buf.String())
	}
	// Warn-once: re-resolving the same invalid value (and a second
	// distinct one, which warns once itself) does not repeat it.
	g.resolveHost("https://example.com/")
	g.resolveHost("https://example.com/")
	g.resolveHost("not a host")
	if warnings := strings.Count(buf.String(), "[gateway]"); warnings != 2 {
		t.Errorf("after re-resolving, warned %d times, want 2 (one per distinct value): %q", warnings, buf.String())
	}
}

func TestGatewayInvalidHostNilLoggerSilent(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "http://example.com:43594/x")
	g := NewGatewayTarget(43594, 242, true, 0, "", nil)
	if got := g.Host(); got != "127.0.0.1" {
		t.Errorf("Host() with invalid PUBLIC_HOST and nil logger = %q, want 127.0.0.1", got)
	}
}

func TestValidBareHost(t *testing.T) {
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"example.com", true},
		{"play-example.com", true},
		{"localhost", true},
		{"", false},
		{"host_name", false},
		{"http://example.com", false},
		{"example.com:43594", false},
		{"example.com/path", false},
		{"exam ple.com", false},
		{"example.com ", false},
	} {
		if got := validBareHost(tc.host); got != tc.want {
			t.Errorf("validBareHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

// decodedWorldList is the worldlist binary decoded field by field.
type decodedWorldList struct {
	length     uint32
	count      uint16
	id         uint16
	flags      uint32
	host       string
	activity   string
	location   uint8
	population uint16
	trailing   int
}

func decodeWorldList(t *testing.T, body []byte) decodedWorldList {
	t.Helper()
	if len(body) < 6 {
		t.Fatalf("worldlist too short: %d bytes", len(body))
	}
	out := decodedWorldList{
		length: binary.BigEndian.Uint32(body[0:4]),
		count:  binary.BigEndian.Uint16(body[4:6]),
	}
	rest := body[6:]
	if int(out.length) != 2+len(rest) {
		t.Errorf("length field = %d, want 2 + record length %d", out.length, 2+len(rest))
	}
	if len(rest) < 9 {
		t.Fatalf("world record too short: %d bytes", len(rest))
	}
	out.id = binary.BigEndian.Uint16(rest[0:2])
	out.flags = binary.BigEndian.Uint32(rest[2:6])
	rest = rest[6:]
	hostEnd := bytes.IndexByte(rest, 0)
	if hostEnd < 0 {
		t.Fatalf("world record has no NUL after host: %q", rest)
	}
	out.host = string(rest[:hostEnd])
	rest = rest[hostEnd+1:]
	actEnd := bytes.IndexByte(rest, 0)
	if actEnd < 0 {
		t.Fatalf("world record has no NUL after activity: %q", rest)
	}
	out.activity = string(rest[:actEnd])
	rest = rest[actEnd+1:]
	if len(rest) != 3 {
		t.Fatalf("world record tail = %d bytes, want 3 (location + population)", len(rest))
	}
	out.location = rest[0]
	out.population = binary.BigEndian.Uint16(rest[1:3])
	out.trailing = 0
	return out
}

func TestGatewayWorldListMembers(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "")
	g := NewGatewayTarget(43594, 242, true, 0, "", nil)
	got := decodeWorldList(t, g.WorldList())
	want := decodedWorldList{count: 1, id: 1, flags: 0x1, host: "127.0.0.1", activity: "tsps", location: 0, population: 0}
	want.length = uint32(2 + (2 + 4 + len(want.host) + 1 + len(want.activity) + 1 + 1 + 2))
	if got != want {
		t.Errorf("worldlist (members) = %+v, want %+v", got, want)
	}
}

func TestGatewayWorldListNonMembers(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "example.com")
	g := NewGatewayTarget(43594, 242, false, 0, "", nil)
	got := decodeWorldList(t, g.WorldList())
	if got.flags != 0 {
		t.Errorf("worldlist (non-members) flags = %#x, want 0", got.flags)
	}
	if got.host != "example.com" || got.activity != "tsps" || got.count != 1 || got.id != 1 {
		t.Errorf("worldlist (non-members) = %+v", got)
	}
}

func TestGatewayWorldListCustomType(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "")
	// A world-type override's flags add to the members bit and its
	// activity replaces "tsps" (GameConstants.setWorldType).
	g := NewGatewayTarget(43594, 242, true, 0x800, "Hardcore", nil)
	got := decodeWorldList(t, g.WorldList())
	if got.flags != 0x801 {
		t.Errorf("worldlist (custom type, members) flags = %#x, want 0x801", got.flags)
	}
	if got.activity != "Hardcore" {
		t.Errorf("worldlist activity = %q, want Hardcore", got.activity)
	}
	g = NewGatewayTarget(43594, 242, false, 0x800, "Hardcore", nil)
	if got := decodeWorldList(t, g.WorldList()); got.flags != 0x800 {
		t.Errorf("worldlist (custom type, non-members) flags = %#x, want 0x800", got.flags)
	}
}

func TestGatewayResolve(t *testing.T) {
	t.Setenv("PUBLIC_HOST", "")
	g := NewGatewayTarget(43594, 242, true, 0, "", nil)

	ct, body, ok := g.Resolve("/gateway/jav_config.ws")
	if !ok {
		t.Fatal("Resolve(jav_config) = not ok, want ok")
	}
	if ct != "text/plain; charset=iso-8859-1" {
		t.Errorf("jav_config content type = %q, want text/plain; charset=iso-8859-1", ct)
	}
	if string(body) != expectedJavConfig("127.0.0.1", 43594, 242) {
		t.Errorf("jav_config body mismatch: %q", body)
	}

	ct, body, ok = g.Resolve("/gateway/worldlist.ws")
	if !ok {
		t.Fatal("Resolve(worldlist) = not ok, want ok")
	}
	if ct != "application/octet-stream" {
		t.Errorf("worldlist content type = %q, want application/octet-stream", ct)
	}
	if got := decodeWorldList(t, body); got.id != 1 || got.count != 1 {
		t.Errorf("worldlist via Resolve = %+v", got)
	}

	// Query strings are stripped before the exact-path match.
	for _, url := range []string{
		"/gateway/jav_config.ws?x=1",
		"/gateway/jav_config.ws?",
		"/gateway/jav_config.ws?x=1/extra",
		"/gateway/worldlist.ws?foo=bar&baz=1",
	} {
		if _, _, ok := g.Resolve(url); !ok {
			t.Errorf("Resolve(%q) = not ok, want ok (query string stripped)", url)
		}
	}

	// Anything else is not a gateway path.
	for _, url := range []string{
		"",
		"/",
		"/gateway",
		"/gateway/jav_config.ws/extra",
		"/gateway/worldlist.ws/extra",
		"/api/world-info",
		"/regions",
		// An escaped path must not decode into a gateway path:
		// TS matches the raw URL string.
		"%2Fgateway%2Fjav_config.ws",
		"/gateway%2Fjav_config.ws",
	} {
		if _, _, ok := g.Resolve(url); ok {
			t.Errorf("Resolve(%q) = ok, want not ok", url)
		}
	}
}
