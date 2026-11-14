package textcontext

import (
	"strings"

	"github.com/trolleyman/ottoman/internal/api"
)

// compactTerminal keeps the caret's logical line and common two-line path
// prompts. Accessibility cannot identify arbitrary shell prompt boundaries.
func compactTerminal(v Snapshot) Snapshot {
	chars := []rune(v.Value)
	left, right := v.Start-v.WindowStart, v.End-v.WindowStart
	if left < 0 || right < left || right > len(chars) {
		return v
	}
	for left > 0 && chars[left-1] != '\n' {
		left--
	}
	for right < len(chars) && chars[right] != '\n' {
		right++
	}
	command := []rune(strings.TrimSpace(string(chars[left:right])))
	if left > 0 && len(command) > 0 && strings.ContainsRune(">❯❮❱➜$#", command[0]) {
		previous := left - 1
		for previous > 0 && chars[previous-1] != '\n' {
			previous--
		}
		line := strings.TrimSpace(string(chars[previous : left-1]))
		if strings.HasPrefix(line, "~/") || strings.HasPrefix(line, "/") {
			left = previous
		}
	}
	v.Value = string(chars[left:right])
	v.WindowStart += left
	var colors []api.TerminalColor
	for _, run := range v.TerminalColors {
		run.Start = max(run.Start, v.WindowStart)
		run.End = min(run.End, v.WindowStart+right-left)
		if run.End > run.Start {
			colors = append(colors, run)
		}
	}
	v.TerminalColors = colors
	return v
}
