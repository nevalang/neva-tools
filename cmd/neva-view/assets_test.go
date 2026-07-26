package main

import "testing"

func TestFSIsAvailableBeforeWebBuild(t *testing.T) {
	assets, err := embeddedUIFS()
	if err != nil {
		t.Fatalf("embeddedUIFS() error: %v", err)
	}
	if _, err := assets.Open("placeholder.txt"); err != nil {
		t.Fatalf("placeholder asset: %v", err)
	}
}
