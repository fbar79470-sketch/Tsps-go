package plugins

import (
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"FreeToPlay", "freetoplay"},
		{"My Plugin!", "myplugin"},
		{"my_plugin", "myplugin"},
		{"MYPLUGIN", "myplugin"},
		{"a-b_c d", "abcd"},
		{"ABC123", "abc123"},
		{"123", "123"},
		{"", ""},
		{"!!!", ""},
		{"Café", "caf"}, // non-ASCII letters are stripped, not folded
		{"CrossFloor", "crossfloor"},
	} {
		if got := NormalizeName(tc.in); got != tc.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveEnabled(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name           string
		switches       map[string]bool
		defaultEnabled *bool
		exported       string
		file           string
		pluginEnabled  *bool
		want           bool
	}{
		{"no switches, no default, nil plugin", nil, nil, "Foo", "Foo", nil, true},
		{"no switches, plugin false", nil, nil, "Foo", "Foo", &no, false},
		{"no switches, plugin true", nil, nil, "Foo", "Foo", &yes, true},
		{"exported true beats plugin false", map[string]bool{"foo": true}, nil, "Foo", "Foo", &no, true},
		{"exported false beats plugin true", map[string]bool{"foo": false}, nil, "Foo", "Foo", &yes, false},
		{"exported beats file", map[string]bool{"exported": true, "file": false}, nil, "Exported", "File", &no, true},
		{"exported false beats file true", map[string]bool{"exported": false, "file": true}, nil, "Exported", "File", &yes, false},
		{"file beats default", map[string]bool{"file": false}, &yes, "Exported", "File", &yes, false},
		{"file true beats default false", map[string]bool{"file": true}, &no, "Exported", "File", &no, true},
		{"default beats plugin", nil, &no, "Foo", "Foo", &yes, false},
		{"default true beats plugin false", nil, &yes, "Foo", "Foo", &no, true},
		{"switch keys are normalized on lookup", map[string]bool{"myplugin": false}, nil, "My Plugin", "Other", &yes, false},
		{"file lookup normalizes too", map[string]bool{"filename": false}, nil, "Exported", "File Name", &yes, false},
		{"empty switches map falls through", map[string]bool{}, nil, "Foo", "Foo", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveEnabled(tc.switches, tc.defaultEnabled, tc.exported, tc.file, tc.pluginEnabled); got != tc.want {
				t.Errorf("ResolveEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseDisabledPlugins(t *testing.T) {
	switches, err := ParseDisabledPlugins([]string{"Foo", "Bar Baz"}, false)
	if err != nil {
		t.Fatalf("ParseDisabledPlugins: %v", err)
	}
	if switches["foo"] || switches["barbaz"] {
		t.Errorf("disabled names must map to false: %v", switches)
	}
	if switches["freetoplay"] {
		t.Errorf("freetoplay must follow the argument (false): %v", switches)
	}

	// FreeToPlay always takes its explicit switch, even when the
	// list names it.
	switches, err = ParseDisabledPlugins([]string{"FreeToPlay"}, true)
	if err != nil {
		t.Fatalf("ParseDisabledPlugins: %v", err)
	}
	if !switches["freetoplay"] {
		t.Errorf("freetoplay must be true: %v", switches)
	}

	switches, err = ParseDisabledPlugins(nil, false)
	if err != nil {
		t.Fatalf("ParseDisabledPlugins: %v", err)
	}
	if len(switches) != 1 || switches["freetoplay"] {
		t.Errorf("empty list yields only freetoplay=false: %v", switches)
	}

	for _, bad := range [][]string{{""}, {"  "}, {"Foo", ""}} {
		if _, err := ParseDisabledPlugins(bad, false); err == nil {
			t.Errorf("ParseDisabledPlugins(%q): expected error", bad)
		} else if !strings.Contains(err.Error(), "disabledPlugins must be a string[]") {
			t.Errorf("ParseDisabledPlugins(%q) error = %q", bad, err)
		}
	}
}
