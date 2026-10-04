// Package tray runs the Windows system-tray icon and menu.
package tray

import (
	_ "embed"
	"fmt"
	"os/exec"
	"sync"
	"syscall"

	"github.com/getlantern/systray"
)

//go:embed assets/tray.ico
var iconBytes []byte

// Options configures the tray.
type Options struct {
	DashboardURL string
	OnQuit       func()
	OnReprobe    func() // tray menu: re-probe availability
	OnShow       func() // tray menu: focus the main window
}

// SetStatus updates the tray tooltip with live state. Safe to call before
// Run: statuses are queued and applied once the tray is ready.
func SetStatus(text string) {
	mu.Lock()
	defer mu.Unlock()
	if statusCh == nil {
		lastStatus = text
		return
	}
	select {
	case statusCh <- text:
	default:
	}
}

var (
	statusCh   chan string
	lastStatus string
	mu         sync.Mutex
)

// Run blocks until the user quits from the tray menu.
func Run(o Options) {
	systray.Run(func() {
		systray.SetIcon(iconBytes)
		systray.SetTitle("Zen Gate")
		systray.SetTooltip("Zen Gate · 本地免费模型网关")
		mu.Lock()
		statusCh = make(chan string, 16)
		pending := lastStatus
		mu.Unlock()
		if pending != "" {
			systray.SetTooltip(pending)
		}
		go func() {
			for t := range statusCh {
				systray.SetTooltip(t)
			}
		}()
		mOpen := systray.AddMenuItem("打开管理页", "Focus the main window")
		mReprobe := systray.AddMenuItem("重新探测可用性", "Re-probe model availability")
		mCopy := systray.AddMenuItem("复制接入地址", "Copy base URL")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出", "Quit zen-gate")
		mOpen.Enable()
		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					if o.OnShow != nil {
						o.OnShow()
						continue
					}
					Open(o.DashboardURL)
				case <-mReprobe.ClickedCh:
					if o.OnReprobe != nil {
						o.OnReprobe()
					}
				case <-mCopy.ClickedCh:
					copyToClipboard(o.DashboardURL)
				case <-mQuit.ClickedCh:
					systray.Quit()
				}
			}
		}()
	}, o.OnQuit)
}

// Open opens the default browser at url.
func Open(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func copyToClipboard(text string) {
	cmd := exec.Command("cmd", "/c", fmt.Sprintf("echo %s| clip", text))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Start()
}
