package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCappedLogWriterKeepsRecentLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jumpd.log")
	w := newCappedLogWriter(path, 3)
	for _, line := range []string{"one\n", "two\n", "three\n", "four\n", "five\n"} {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	want := "three\nfour\nfive\n"
	if got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCappedLogWriterHandlesMultiLineWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jumpd.log")
	w := newCappedLogWriter(path, 2)
	if _, err := w.Write([]byte("one\ntwo\nthree\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "two\nthree\n" {
		t.Fatalf("log = %q", got)
	}
}

func TestCappedLogWriterKeepsPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jumpd.log")
	w := newCappedLogWriter(path, 2)
	if _, err := w.Write([]byte("one\nt")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("wo\nthree\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "two\nthree\n") {
		t.Fatalf("log = %q", string(data))
	}
}
