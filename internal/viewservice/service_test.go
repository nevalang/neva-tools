package viewservice

import (
	"testing"

	"github.com/nevalang/neva/pkg/view"
)

func TestFilterProgramModules(t *testing.T) {
	includeCurrent := true
	includeDeps := false
	includeStd := false

	program := view.Program{Modules: []view.Module{
		{Path: "@"},
		{Path: "github.com/example/dependency"},
		{Path: "std"},
	}}

	got := FilterProgramModules(program, ProgramRequest{
		IncludeCurrent: &includeCurrent,
		IncludeDeps:    &includeDeps,
		IncludeStd:     &includeStd,
	})
	if len(got.Modules) != 1 || got.Modules[0].Path != "@" {
		t.Fatalf("filtered modules = %#v, want current module only", got.Modules)
	}
}
