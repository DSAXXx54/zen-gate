//go:build windows

package window

import (
	"github.com/jchv/go-webview2"

	"zen-gate/internal/tray"
)

// Run creates the window, navigates to the URL and blocks until the window is
// closed. Must be called from the main thread: the WebView2 message pump owns
// it, while the tray runs on its own locked goroutine.
func Run(o Options) {
	setProcessDPIAwareness()

	// The tray must come up before the window so a close-to-tray app never
	// flashes a taskbar-less state.
	if o.Tray != nil {
		t := *o.Tray
		go func() {
			tray.Register(tray.Options{
				DashboardURL: t.DashboardURL,
				OnQuit:       t.OnQuit,
				OnReprobe:    t.OnReprobe,
				OnShow:       t.OnShow,
			})
			tray.Loop()
		}()
	}

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		DataPath:  o.DataPath,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  o.Title,
			Width:  uint(o.Width),
			Height: uint(o.Height),
			Center: true,
			IconId: 2,
		},
	})
	if w == nil {
		return
	}
	defer w.Destroy()

	hwnd := uintptr(w.Window())
	if hwnd != 0 {
		themeTitleBar(hwnd)  // dark DWM border, no light 1px frame
		stripCaption(hwnd)   // the title bar is drawn by the UI itself
		setWindowIcons(hwnd) // taskbar + window icon (runtime WM_SETICON)
		if o.Bounds != nil && o.Bounds.W > 0 {
			w, h := o.Bounds.W, o.Bounds.H
			// A save that happened while minimized can persist the collapsed
			// titlebar size; fall back to the defaults rather than opening as
			// a tiny sliver.
			if w < 400 || h < 300 {
				w, h = o.Width, o.Height
			}
			ApplyBounds(hwnd, o.Bounds.X, o.Bounds.Y, w, h)
		}
		bindWindowControls(w, hwnd, o.OnCloseButton)
		if o.OnReady != nil {
			o.OnReady(hwnd)
		}
		if o.Bounds != nil && o.Bounds.Maximized {
			MaximizeWindow(hwnd)
		}
	}

	w.SetTitle(o.Title)
	w.Navigate(o.URL)
	w.Run()
}

// bindWindowControls exposes the custom title bar's actions to JS. The HTML
// title bar calls these: drag on mousedown, buttons for min/max/close.
func bindWindowControls(w webview2.WebView, hwnd uintptr, onCloseButton func() bool) {
	_ = w.Bind("zengateDrag", func() error {
		startDrag(hwnd)
		return nil
	})
	_ = w.Bind("zengateMinimize", func() error {
		minimizeWindow(hwnd)
		return nil
	})
	_ = w.Bind("zengateToggleMax", func() error {
		toggleMaximize(hwnd)
		return nil
	})
	_ = w.Bind("zengateClose", func() error {
		if onCloseButton != nil && !onCloseButton() {
			return nil // close-to-tray: the window stays, just hidden by caller
		}
		closeWindow(hwnd)
		return nil
	})
}
