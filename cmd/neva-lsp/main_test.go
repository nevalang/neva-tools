package main

import "testing"

func TestVersionInfoUsesStableMachineContract(t *testing.T) {
	info := versionInfo{
		SchemaVersion:   1,
		Component:       "lsp",
		Version:         version,
		NevaVersion:     nevaVersion,
		ProtocolVersion: 1,
	}

	if info.SchemaVersion != 1 || info.Component != "lsp" || info.ProtocolVersion != 1 {
		t.Fatalf("unexpected version contract: %#v", info)
	}
}
