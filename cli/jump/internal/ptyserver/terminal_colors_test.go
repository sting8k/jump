package ptyserver

import "testing"

func TestTerminalColorTrackerBackgroundReplay(t *testing.T) {
	var tracker terminalColorTracker

	if got := tracker.backgroundReplaySeq(); got != "\x1b]111\x07" {
		t.Fatalf("empty replay seq = %q, want OSC 111 restore", got)
	}

	tracker.write([]byte("before\x1b]11;rgb:12/34/56\x07after"))
	if got := tracker.backgroundReplaySeq(); got != "\x1b]11;rgb:12/34/56\x07" {
		t.Fatalf("set replay seq = %q", got)
	}

	tracker.write([]byte("\x1b]111\x07"))
	if got := tracker.backgroundReplaySeq(); got != "\x1b]111\x07" {
		t.Fatalf("restore replay seq = %q, want OSC 111 restore", got)
	}
}

func TestTerminalColorTrackerHandlesSplitOSC(t *testing.T) {
	var tracker terminalColorTracker

	tracker.write([]byte("\x1b]11;rgb:12"))
	if got := tracker.backgroundReplaySeq(); got != "\x1b]111\x07" {
		t.Fatalf("partial OSC changed replay seq: %q", got)
	}

	tracker.write([]byte("/34/56\x07"))
	if got := tracker.backgroundReplaySeq(); got != "\x1b]11;rgb:12/34/56\x07" {
		t.Fatalf("split set replay seq = %q", got)
	}
}

func TestTerminalColorTrackerIgnoresReportsAndKeepsPreviousColor(t *testing.T) {
	var tracker terminalColorTracker
	tracker.write([]byte("\x1b]11;#123456\x07"))
	tracker.write([]byte("\x1b]11;?\x07"))

	if got := tracker.backgroundReplaySeq(); got != "\x1b]11;#123456\x07" {
		t.Fatalf("report changed replay seq: %q", got)
	}
}
