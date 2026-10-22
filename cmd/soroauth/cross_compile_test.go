package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlaceBinaryCopiesToDestinationAndRemovesSource(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	destinationDir := filepath.Join(root, "destination")
	if err := os.Mkdir(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(destinationDir, 0o700); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(sourceDir, "soroauth")
	destination := filepath.Join(destinationDir, "soroauth")
	want := []byte("binary fixture")
	if err := os.WriteFile(source, want, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := placeBinary(source, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("destination contents = %q, want %q", got, want)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Errorf("source still exists or could not be checked: %v", err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("destination permissions = %04o, want 0755", got)
	}
}
