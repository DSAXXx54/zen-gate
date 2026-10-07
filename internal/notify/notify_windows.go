//go:build windows

// Windows toast notifications, delivered through the PowerShell WinRT channel
// with the subprocess console hidden.
package notify

import (
	"os/exec"
	"strings"
	"syscall"
)

// Toast shows a Windows toast with app name "Zen Gate".
// Best-effort: PowerShell delivery failures are silently ignored.
func Toast(title, body string) {
	title, body, ok := want(title, body)
	if !ok {
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
