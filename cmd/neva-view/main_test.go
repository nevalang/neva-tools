package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStandaloneListenAddr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		port    int
		want    string
		wantErr bool
	}{
		{name: "valid", port: 7788, want: "127.0.0.1:7788"},
		{name: "lower bound", port: 1, want: "127.0.0.1:1"},
		{name: "upper bound", port: 65535, want: "127.0.0.1:65535"},
		{name: "too small", port: 0, wantErr: true},
		{name: "too large", port: 65536, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := standaloneListenAddr(tc.port)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("standaloneListenAddr(%d) error=nil, want error", tc.port)
				}
				return
			}
			if err != nil {
				t.Fatalf("standaloneListenAddr(%d) unexpected error: %v", tc.port, err)
			}
			if got != tc.want {
				t.Fatalf("standaloneListenAddr(%d)=%q, want %q", tc.port, got, tc.want)
			}
		})
	}
}

func TestResolveWorkspacePath(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	got, err := resolveWorkspacePath(tempDir)
	if err != nil {
		t.Fatalf("resolveWorkspacePath(%q) unexpected error: %v", tempDir, err)
	}
	wantAbs, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatalf("filepath.Abs(%q) unexpected error: %v", tempDir, err)
	}
	if got != wantAbs {
		t.Fatalf("resolveWorkspacePath(%q)=%q, want %q", tempDir, got, wantAbs)
	}

	filePath := filepath.Join(tempDir, "not-a-dir.txt")
	if writeErr := os.WriteFile(filePath, []byte("x"), 0o644); writeErr != nil {
		t.Fatalf("os.WriteFile(%q) unexpected error: %v", filePath, writeErr)
	}
	if _, err := resolveWorkspacePath(filePath); err == nil {
		t.Fatalf("resolveWorkspacePath(%q) error=nil, want error for non-directory path", filePath)
	}

	missingPath := filepath.Join(tempDir, "missing-dir")
	if _, err := resolveWorkspacePath(missingPath); err == nil {
		t.Fatalf("resolveWorkspacePath(%q) error=nil, want error for missing directory", missingPath)
	}
}
