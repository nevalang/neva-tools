// neva-lsp starts the Neva Language Server Protocol implementation.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/nevalang/neva-tools/internal/lsp"
)

// version and nevaVersion are set by the component-release workflow.
var (
	version     = "dev"
	nevaVersion = "dev"
)

type versionInfo struct {
	SchemaVersion   int    `json:"schemaVersion"`
	Component       string `json:"component"`
	Version         string `json:"version"`
	NevaVersion     string `json:"nevaVersion"`
	ProtocolVersion int    `json:"protocolVersion"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		printVersion(os.Args[2:])
		return
	}

	debug := flag.Bool("debug", false, "run the server over TCP on localhost:6007")
	flag.Parse()
	if err := lsp.Run(*debug); err != nil {
		panic(err)
	}
}

func printVersion(args []string) {
	flags := flag.NewFlagSet("neva-lsp version", flag.ExitOnError)
	asJSON := flags.Bool("json", false, "write machine-readable version information")
	_ = flags.Parse(args)

	info := versionInfo{
		SchemaVersion:   1,
		Component:       "lsp",
		Version:         version,
		NevaVersion:     nevaVersion,
		ProtocolVersion: 1,
	}
	if !*asJSON {
		fmt.Println(info.Version)
		return
	}

	if err := json.NewEncoder(os.Stdout).Encode(info); err != nil {
		panic(err)
	}
}
