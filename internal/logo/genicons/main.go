// Command genicons writes the logo assets from internal/logo (`make icons`,
// run from the repo root): build/windows/icon.ico (exe and installer icon),
// build/appicon.png (1024×1024) and frontend/public/logo.svg (favicon). The
// outputs are committed; internal/logo tests fail when they drift.
package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"github.com/jacopofilonzi/wchron/internal/logo"
)

// AppIconSize is the side of build/appicon.png.
const AppIconSize = 1024

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "genicons:", err)
		os.Exit(1)
	}
}

func run() error {
	ico, err := logo.ICO()
	if err != nil {
		return err
	}
	var app bytes.Buffer
	if err := png.Encode(&app, logo.Draw(AppIconSize)); err != nil {
		return fmt.Errorf("encode appicon: %w", err)
	}
	files := []struct {
		path string
		data []byte
	}{
		{filepath.Join("build", "windows", "icon.ico"), ico},
		{filepath.Join("build", "appicon.png"), app.Bytes()},
		{filepath.Join("frontend", "public", "logo.svg"), []byte(logo.SVG)},
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", f.path, err)
		}
		if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
		fmt.Println("wrote", f.path)
	}
	return nil
}
