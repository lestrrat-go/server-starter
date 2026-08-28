package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status")
	if err := os.WriteFile(path, []byte("2:200\n1:100\n"), 0600); err != nil {
		t.Fatal(err)
	}
	status, err := readStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if status[1] != 100 || status[2] != 200 {
		t.Fatalf("status = %#v", status)
	}
	if !generationAdvanced(map[int]int{1: 100}, status) {
		t.Fatal("status did not advance")
	}
	if !oldWorkersGone(map[int]int{1: 100}, map[int]int{2: 200}) {
		t.Fatal("old worker was reported as alive")
	}
}
