package main

import "testing"

func TestValidateDestination(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
		ok   bool
	}{
		{"https", "https://example.com/path", true},
		{"http", "http://example.com", true},
		{"relative", "/internal", false},
		{"javascript", "javascript:alert(1)", false},
		{"localhost", "http://localhost/admin", false},
		{"loopback", "http://127.0.0.1/admin", false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := validateDestination(test.url) == nil; got != test.ok {
				t.Fatalf("validateDestination(%q) valid=%v, want %v", test.url, got, test.ok)
			}
		})
	}
}

func TestAliasValidation(t *testing.T) {
	t.Parallel()
	for value, expected := range map[string]bool{"launch-2026": true, "ab": false, "HasCaps": false, "bad/path": false} {
		if validAlias(value) != expected {
			t.Errorf("validAlias(%q)=%v, want %v", value, !expected, expected)
		}
	}
}

func TestReservedAliases(t *testing.T) {
	for _, alias := range []string{"live", "ready", "metrics"} {
		if validAlias(alias) {
			t.Errorf("reserved alias %s accepted", alias)
		}
	}
}
