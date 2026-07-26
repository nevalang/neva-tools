package main

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyResolveFile_NotRegisteredInHandlerDispatch(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("read handler.go: %v", err)
	}
	if strings.Contains(string(content), "resolve_file") {
		t.Fatal("legacy resolve_file must not be present in handler dispatch")
	}
}
