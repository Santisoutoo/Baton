package config

import "path/filepath"

// UsageDBPath is the SQLite usage database, kept next to the global config file
// (~/.config/batuta/usage.db).
func UsageDBPath() string {
	gp := GlobalPath()
	if gp == "" {
		return "usage.db"
	}
	return filepath.Join(filepath.Dir(gp), "usage.db")
}
