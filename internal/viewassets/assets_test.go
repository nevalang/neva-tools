package viewassets

import "testing"

func TestFSIsAvailableBeforeWebBuild(t *testing.T) {
	assets, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	if _, err := assets.Open("placeholder.txt"); err != nil {
		t.Fatalf("placeholder asset: %v", err)
	}
}
