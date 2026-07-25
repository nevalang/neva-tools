// neva-view hosts the standalone Neva visual editor.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nevalang/neva-lsp/internal/viewassets"
	"github.com/nevalang/neva-lsp/internal/viewserver"
	"github.com/tliron/commonlog"
	_ "github.com/tliron/commonlog/simple"
)

func main() {
	port := flag.Int("port", 7788, "HTTP port")
	open := flag.Bool("open", false, "open the editor in a browser")
	workspace := flag.String("workspace", ".", "Neva workspace directory")
	flag.Parse()
	if *port < 1 || *port > 65535 {
		panic(fmt.Errorf("port must be in range [1,65535], got %d", *port))
	}
	path, err := filepath.Abs(filepath.Clean(*workspace))
	if err != nil {
		panic(err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		panic(fmt.Errorf("workspace is not an accessible directory: %s", path))
	}
	ui, err := viewassets.FS()
	if err != nil {
		panic(fmt.Errorf("load visual-editor UI: %w", err))
	}
	commonlog.Configure(1, nil)
	if err := viewserver.Run(commonlog.GetLogger("neva.view"), viewserver.Config{WorkspacePath: path, ListenAddr: fmt.Sprintf("127.0.0.1:%d", *port), OpenBrowser: *open, UI: ui}); err != nil {
		panic(err)
	}
}
