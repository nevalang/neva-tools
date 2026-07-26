// neva-view hosts the standalone Neva visual editor.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tliron/commonlog"
	_ "github.com/tliron/commonlog/simple"
)

func main() {
	port := flag.Int("port", 7788, "HTTP port")
	open := flag.Bool("open", false, "open the editor in a browser")
	workspace := flag.String("workspace", ".", "Neva workspace directory")
	flag.Parse()
	listenAddr, err := standaloneListenAddr(*port)
	if err != nil {
		panic(err)
	}
	path, err := resolveWorkspacePath(*workspace)
	if err != nil {
		panic(err)
	}
	ui, err := embeddedUIFS()
	if err != nil {
		panic(fmt.Errorf("load visual-editor UI: %w", err))
	}
	commonlog.Configure(1, nil)
	if err := runView(commonlog.GetLogger("neva.view"), viewConfig{WorkspacePath: path, ListenAddr: listenAddr, OpenBrowser: *open, UI: ui}); err != nil {
		panic(err)
	}
}

func standaloneListenAddr(port int) (string, error) {
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be in range [1,65535], got %d", port)
	}
	return fmt.Sprintf("127.0.0.1:%d", port), nil
}

func resolveWorkspacePath(raw string) (string, error) {
	path := filepath.Clean(raw)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("workspace is not accessible: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace must be a directory: %s", path)
	}
	return filepath.Abs(path)
}
