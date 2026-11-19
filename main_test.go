package main

import (
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want options
	}{
		{"none", nil, options{}},
		{"toggle", []string{"--toggle"}, options{toggle: true}},
		{"version", []string{"--version"}, options{version: true}},
		{"version short", []string{"-V"}, options{version: true}},
		{"help", []string{"--help"}, options{help: true}},
		{"help short", []string{"-h"}, options{help: true}},
		{"setup", []string{"--setup"}, options{setup: true}},
		{"setup install", []string{"--setup", "--install"}, options{setup: true, install: true}},
		{"daemon", []string{"--daemon"}, options{daemon: true}},
		{"verbose", []string{"--verbose"}, options{verbose: true}},
		{"verbose short", []string{"-v"}, options{verbose: true}},
		{"device", []string{"--device", "/dev/input/event5"}, options{device: "/dev/input/event5"}},
		{"device short", []string{"-d", "/dev/input/event5"}, options{device: "/dev/input/event5"}},
		{"bare path is device", []string{"/dev/input/event7"}, options{device: "/dev/input/event7"}},
		{"theme", []string{"--theme", "nord"}, options{theme: "nord"}},
		{"theme short", []string{"-t", "nord"}, options{theme: "nord"}},
		{"config", []string{"--config", "/tmp/c"}, options{config: "/tmp/c"}},
		{"config short", []string{"-c", "/tmp/c"}, options{config: "/tmp/c"}},
		{"value may start with a dash", []string{"-t", "-v"}, options{theme: "-v"}},
		{"help after config keeps both", []string{"-c", "/tmp/c", "--help"}, options{config: "/tmp/c", help: true}},
		{"version after an option", []string{"--daemon", "-V"}, options{daemon: true, version: true}},
		{"later device wins", []string{"-d", "/a", "/b"}, options{device: "/b"}},
		{"all", []string{"--daemon", "-v", "-d", "/d", "-t", "nord", "-c", "/c", "--toggle"},
			options{daemon: true, verbose: true, device: "/d", theme: "nord", config: "/c", toggle: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if err != nil {
				t.Fatalf("parseArgs(%q) error: %v", tt.args, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseArgs(%q) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseArgsMissingValue(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--device"}, "--device requires a path"},
		{[]string{"-d"}, "--device requires a path"},
		{[]string{"--theme"}, "--theme requires a name"},
		{[]string{"-t"}, "--theme requires a name"},
		{[]string{"--config"}, "--config requires a path"},
		{[]string{"-v", "-c"}, "--config requires a path"},
	}
	for _, tt := range tests {
		_, err := parseArgs(tt.args)
		if err == nil || err.Error() != tt.want {
			t.Errorf("parseArgs(%q) error = %v, want %q", tt.args, err, tt.want)
		}
	}
}

func TestParseArgsUnknown(t *testing.T) {
	got, err := parseArgs([]string{"--confg", "x", "--daemon", "-q"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.daemon {
		t.Error("an unknown argument stopped parsing of the rest")
	}
	if want := []string{"--confg", "x", "-q"}; !reflect.DeepEqual(got.unknown, want) {
		t.Errorf("unknown = %q, want %q", got.unknown, want)
	}
}

func TestUnknownArgWarning(t *testing.T) {
	tests := []struct{ arg, want string }{
		{"--confg", `Warning: unknown option "--confg", ignored (did you mean --config?)`},
		{"-config", `Warning: unknown option "-config", ignored (did you mean --config?)`},
		{"--deamon", `Warning: unknown option "--deamon", ignored (did you mean --daemon?)`},
		{"--them", `Warning: unknown option "--them", ignored (did you mean --theme?)`},
		{"--fullscreen", `Warning: unknown option "--fullscreen", ignored`},
		{"-q", `Warning: unknown option "-q", ignored`},
		{"x", `Warning: unknown option "x", ignored`},
	}
	for _, tt := range tests {
		if got := unknownArgWarning(tt.arg); got != tt.want {
			t.Errorf("unknownArgWarning(%q) = %q, want %q", tt.arg, got, tt.want)
		}
	}
}

func TestEditDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"abc", "", 3}, {"", "ab", 2}, {"config", "config", 0},
		{"confg", "config", 1}, {"deamon", "daemon", 2}, {"kitten", "sitting", 3},
	}
	for _, tt := range tests {
		if got := editDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestWithRevision(t *testing.T) {
	for _, tt := range []struct {
		rev      string
		modified bool
		want     string
	}{
		{"", false, "2.1.1"},
		{"0d08c62f1a2b3c4d5e6f", false, "2.1.1+git.0d08c62"},
		{"0d08c62f1a2b3c4d5e6f", true, "2.1.1+git.0d08c62.modified"},
		{"abc", false, "2.1.1+git.abc"},
	} {
		if got := withRevision("2.1.1", tt.rev, tt.modified); got != tt.want {
			t.Errorf("withRevision(%q, %v) = %q, want %q", tt.rev, tt.modified, got, tt.want)
		}
	}
}
