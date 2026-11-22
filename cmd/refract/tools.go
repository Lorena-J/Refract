package main

import (
	"log/slog"

	"github.com/ponytojas/Refract/internal/tools"
)

// logTools logs where each external program was found and what won't work
// without the ones that are missing. Nothing stops the server: the web app
// shows the missing ones (/api/me) instead of failing on first use.
func logTools() (missing []string) {
	for _, t := range tools.All() {
		if t.Found() {
			slog.Info("tool", "name", t.Name, "path", t.Path, "source", t.Source)
			continue
		}
		missing = append(missing, t.Name)
		slog.Error("tool missing", "name", t.Name, "without_it", tools.Features[t.Name]+" won't work", "fix", tools.Hint(t.Name))
	}
	return missing
}
