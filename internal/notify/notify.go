// Package notify sends Windows toast notifications (PowerShell WinRT channel,
// subprocess console hidden). Notifications are fired only for events the user
// can't already see in a focused window.
package notify

import (
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

var (
	mu       sync.Mutex
	disabled bool
)

// SetEnabled toggles the whole channel (settings switch).
func SetEnabled(on bool) {
	mu.Lock()
	disabled = !on
	mu.Unlock()
}

// Enabled reports current state.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return !disabled
}

// Toast shows a Windows toast with app name "Zen Gate".
// Best-effort: PowerShell delivery failures are silently ignored.
func Toast(title, body string) {
	if !Enabled() || strings.TrimSpace(title) == "" {
		return
	}
	ps := buildScript(title, body)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Start()
}

func esc(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "'", "''"), "`", "``")
}

func buildScript(title, body string) string {
	return `
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null;
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null;
$xml = @"<toast><visual><binding template="ToastGeneric"><text>` + esc(title) + `</text><text>` + esc(body) + `</text></binding></visual></toast>"@;
$doc = New-Object Windows.Data.Xml.Dom.XmlDocument;
$doc.LoadXml($xml);
$t = [Windows.UI.Notifications.ToastNotification]::new($doc);
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("Zen Gate").Show($t);`
}
