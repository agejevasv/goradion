package main

import (
	"flag"
	"testing"
)

func TestRemoteFlag(t *testing.T) {
	key := func(s string) *string { return &s }
	for _, tc := range []struct {
		args []string
		on   bool
		key  *string
		rest []string
	}{
		{args: nil},
		{args: []string{"-d"}},
		{args: []string{"-r"}, on: true},
		{args: []string{"-r", "-d"}, on: true},
		{args: []string{"-d", "-r"}, on: true},
		{args: []string{"-r", "true"}, on: true, key: key("true")},
		{args: []string{"-r", "Secret Phrase"}, on: true, key: key("Secret Phrase")},
		{args: []string{"--r", "abc", "-p", "8080"}, on: true, key: key("abc")},
		{args: []string{"-r", ""}, on: true, key: key("")},
		{args: []string{"-r="}, on: true, key: key("")},
		{args: []string{"-r=-dash"}, on: true, key: key("-dash")},
		{args: []string{"-s", "-r"}},
		{args: []string{"-s", "x.csv", "-r", "k"}, on: true, key: key("k")},
		{args: []string{"--", "-r", "k"}, rest: []string{"-r", "k"}},
	} {
		remote = remoteFlag{}
		if err := flag.CommandLine.Parse(markOptionalValue(tc.args, "r")); err != nil {
			t.Fatalf("%q: %v", tc.args, err)
		}
		if remote.on != tc.on || (remote.key == nil) != (tc.key == nil) || (tc.key != nil && *remote.key != *tc.key) {
			t.Errorf("%q: got on=%v key=%v, want on=%v key=%v", tc.args, remote.on, deref(remote.key), tc.on, deref(tc.key))
		}
		if got := flag.Args(); len(got) != len(tc.rest) {
			t.Errorf("%q: args left %q, want %q", tc.args, got, tc.rest)
		}
	}
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
