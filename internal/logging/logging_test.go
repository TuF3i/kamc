package logging

import (
	"testing"
)

func TestParseLevel(t *testing.T) {
	for raw, want := range map[string]string{
		"":        "info",
		"debug":   "debug",
		"WARN":    "warn",
		"error":   "error",
		"warning": "warn",
	} {
		level, err := parseLevel(raw)
		if err != nil {
			t.Fatalf("parseLevel(%q): %v", raw, err)
		}
		if level.String() != want {
			t.Fatalf("parseLevel(%q): want %s, got %s", raw, want, level)
		}
	}
	if _, err := parseLevel("verbose"); err == nil {
		t.Fatal("unknown level should fail")
	}
}
