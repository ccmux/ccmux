package bot

import "github.com/ccmux/ccmux/internal/monitor"

func renderEntry(entry monitor.ParsedEntry) string {
	return entry.Text
}
