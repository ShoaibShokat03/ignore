package ui

import (
	"os"

	"ignore/internal/app"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func Run(a *app.Ignore) error {
	return wails.Run(&options.App{
		Title:  "Ignore",
		Width:  980,
		Height: 680,
		// Closing the window only hides it; the app keeps running in the
		// background (system tray) so clipboard/copy-paste protection stays
		// active after the user closes the editor window. Use the tray's
		// "Exit" item to fully quit.
		HideWindowOnClose: true,
		// When launched from the Windows startup entry (with --background), start
		// minimised to the tray so protection runs in the background without
		// popping the window open at login. A normal manual launch shows it.
		StartHidden: launchedInBackground(),
		// Enforce a single running instance. Protection is done by one background
		// clipboard monitor that owns the clipboard; a second instance would start
		// a second monitor and the two would fight over the clipboard (the
		// "works when closed but not when open" symptom). With this lock, launching
		// the app again simply reveals the already-running instance's window.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "Ignore-File-Filter-SingleInstance-7c9e6679f1",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				a.ShowWindow()
			},
		},
		AssetServer: &assetserver.Options{
			Assets: a.Assets(),
		},
		BackgroundColour: &options.RGBA{R: 18, G: 22, B: 28, A: 1},
		OnStartup:        a.Startup,
		OnShutdown:       a.Shutdown,
		Bind: []interface{}{
			a,
		},
	})
}

// launchedInBackground reports whether the process was started by the Windows
// startup entry, signalled by a background flag on the command line.
func launchedInBackground() bool {
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--background", "--hidden", "--startup", "-background", "/background":
			return true
		}
	}
	return false
}
