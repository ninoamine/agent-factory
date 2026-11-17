package main

import "testing"

func TestHistoryMutates(t *testing.T) { // TUI-15 relies on this
	b := &backend{}
	for args, want := range map[string]bool{
		"": false, "delete abc": true, "clear": true, "list": false, "  clear ": true,
	} {
		if got := b.HistoryMutates(args); got != want {
			t.Errorf("HistoryMutates(%q) = %v, want %v", args, got, want)
		}
	}
}
