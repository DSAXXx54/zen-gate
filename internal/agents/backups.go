package agents

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"zen-gate/internal/store"
)

// backupFile stores a pre-injection copy of an agent config under
// <zen-gate home>/backups/<agent>/.
func backupFile(agentID, path string, data []byte) (string, error) {
	dir := filepath.Join(store.Home(), "backups", agentID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := time.Now().Format("20060102-150405") + "-" + filepath.Base(path)
	p := filepath.Join(dir, name)
	return p, os.WriteFile(p, data, 0o600)
}

// latestBackup returns the newest backup of a given base filename, if any.
func latestBackup(agentID, base string) string {
	dir := filepath.Join(store.Home(), "backups", agentID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	names := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), base) {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return filepath.Join(dir, names[0])
}
