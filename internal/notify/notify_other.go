//go:build !windows && !darwin

package notify

// Toast is a no-op where no notification backend is wired up.
func Toast(title, body string) {
	_, _, _ = want(title, body)
}
