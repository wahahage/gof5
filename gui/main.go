package main

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if runtime.GOOS == "darwin" {
		if os.Geteuid() != 0 {
			ex, err := os.Executable()
			if err != nil {
				println("Error getting executable path:", err.Error())
				return
			}

			// Construct osascript command to re-launch with admin privileges
			// usage: do shell script "..." with administrator privileges
			// We append " &> /dev/null &" to run in background so osascript doesn't block
			// and the parent process can exit immediately.
			script := fmt.Sprintf("do shell script \"%s &> /dev/null &\" with administrator privileges with prompt \"GoF5 VPN needs administrator privileges to configure network settings.\"", ex)
			cmd := exec.Command("osascript", "-e", script)

			output, err := cmd.CombinedOutput()
			if err != nil {
				println("Error requesting admin privileges:", err.Error(), "\nOutput:", string(output))
				return
			}
			// If successful, exit the non-privileged instance
			os.Exit(0)
		}
	}
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "GoF5 VPN",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.BeforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
