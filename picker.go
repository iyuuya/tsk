package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
)

// selectTask is replaceable by tests so command behavior can be exercised
// without opening the interactive picker.
var selectTask = selectInteractively

// pickerRows returns tab-separated picker rows. The leading index identifies
// the selected row without re-parsing fields that may themselves contain
// spaces.
func pickerRows(rows []listTask, global bool) []string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		location := row.Dir
		if global {
			location = projectLabel(row.Root) + ":" + location
		}
		line := fmt.Sprintf("%d\t%s\t%s\t%s", i, location, row.Adaptor, row.Task)
		if row.Description != "" {
			line += "\t" + row.Description
		}
		lines[i] = line
	}
	return lines
}

// selectInteractively is a self-contained fuzzy task picker. It does not
// require fzf or another external executable.
func selectInteractively(rows []string) (int, error) {
	in, out := os.Stdin, os.Stderr
	if !term.IsTerminal(int(in.Fd())) || !term.IsTerminal(int(out.Fd())) {
		return -1, fmt.Errorf("interactive task selection requires a terminal")
	}

	oldState, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return -1, fmt.Errorf("enable interactive task selection: %w", err)
	}
	defer term.Restore(int(in.Fd()), oldState)

	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l")
	defer fmt.Fprint(out, "\x1b[?25h\x1b[?1049l")

	query := []rune{}
	cursor := 0
	reader := bufio.NewReader(in)
	for {
		matches := fuzzyMatches(rows, string(query))
		if cursor >= len(matches) {
			cursor = len(matches) - 1
		}
		if cursor < 0 {
			cursor = 0
		}
		renderPicker(out, string(query), rows, matches, cursor)

		key, err := readPickerKey(reader)
		if err == io.EOF {
			return -1, nil
		}
		if err != nil {
			return -1, fmt.Errorf("read task selection: %w", err)
		}
		switch key {
		case "enter":
			if len(matches) > 0 {
				return matches[cursor].index, nil
			}
		case "cancel":
			return -1, nil
		case "up":
			if cursor > 0 {
				cursor--
			}
		case "down":
			if cursor+1 < len(matches) {
				cursor++
			}
		case "backspace":
			if len(query) > 0 {
				query = query[:len(query)-1]
			}
		default:
			query = append(query, []rune(key)...)
			cursor = 0
		}
	}
}

type pickerMatch struct {
	index int
	score int
}

// fuzzyMatches matches query runes in order, like fzf's default fuzzy mode.
// Earlier and more contiguous matches sort before scattered matches.
func fuzzyMatches(rows []string, query string) []pickerMatch {
	queryRunes := []rune(strings.ToLower(query))
	matches := make([]pickerMatch, 0, len(rows))
	for index, row := range rows {
		_, label, _ := strings.Cut(row, "\t")
		text := []rune(strings.ToLower(label))
		position, score := 0, 0
		matched := true
		for _, queryRune := range queryRunes {
			found := -1
			for i := position; i < len(text); i++ {
				if text[i] == queryRune {
					found = i
					break
				}
			}
			if found == -1 {
				matched = false
				break
			}
			score += found - position
			position = found + 1
		}
		if matched {
			matches = append(matches, pickerMatch{index: index, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score < matches[j].score
	})
	return matches
}

func renderPicker(out io.Writer, query string, rows []string, matches []pickerMatch, cursor int) {
	width, height, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil {
		width = 80
		height = 24
	}
	limit := max(height-4, 1)
	start := max(cursor-limit+1, 0)
	end := min(start+limit, len(matches))
	columns := pickerColumns(rows, width)

	fmt.Fprint(out, "\x1b[2J\x1b[H")
	fmt.Fprint(out, "\x1b[1;36m tsk run \x1b[0m  \x1b[2m検索して Enter で実行  Ctrl-C/Esc で中止\x1b[0m\r\n")
	fmt.Fprintf(out, "\x1b[36m > \x1b[0m%s  \x1b[2m%d/%d 件\x1b[0m\r\n\r\n", truncateDisplay(query, max(width-16, 1)), len(matches), len(rows))
	fmt.Fprintf(out, "\x1b[2m  %s\x1b[0m\r\n", formatPickerFields("DIR", "ADAPTOR", "TASK", "DESCRIPTION", columns))
	for i := start; i < end; i++ {
		match := matches[i]
		label := formatPickerRow(rows[match.index], columns)
		if i == cursor {
			fmt.Fprintf(out, "\x1b[7m> %s\x1b[0m\r\n", label)
		} else {
			fmt.Fprintf(out, "  %s\r\n", label)
		}
	}
}

type pickerColumnWidths struct {
	dir         int
	adaptor     int
	task        int
	description int
}

func pickerColumns(rows []string, width int) pickerColumnWidths {
	var maxDir, maxAdaptor, maxTask int
	for _, row := range rows {
		dir, adaptor, task, _ := pickerRowFields(row)
		maxDir = max(maxDir, displayWidth(dir))
		maxAdaptor = max(maxAdaptor, displayWidth(adaptor))
		maxTask = max(maxTask, displayWidth(task))
	}

	columns := pickerColumnWidths{
		dir:     min(max(maxDir, displayWidth("DIR")), 24),
		adaptor: min(max(maxAdaptor, displayWidth("ADAPTOR")), 14),
		task:    min(max(maxTask, displayWidth("TASK")), 24),
	}
	available := max(width-2, 1)
	for columns.dir+columns.adaptor+columns.task+6 > available {
		switch {
		case columns.task > 8:
			columns.task--
		case columns.dir > 6:
			columns.dir--
		case columns.adaptor > 6:
			columns.adaptor--
		default:
			return columns
		}
	}
	columns.description = available - columns.dir - columns.adaptor - columns.task - 6
	return columns
}

func formatPickerRow(row string, columns pickerColumnWidths) string {
	dir, adaptor, task, description := pickerRowFields(row)
	return formatPickerFields(dir, adaptor, task, description, columns)
}

func formatPickerFields(dir, adaptor, task, description string, columns pickerColumnWidths) string {
	line := padDisplay(dir, columns.dir) + "  " +
		padDisplay(adaptor, columns.adaptor) + "  " +
		padDisplay(task, columns.task)
	if columns.description > 0 {
		line += "  " + truncateDisplay(description, columns.description)
	}
	return line
}

func pickerRowFields(row string) (dir, adaptor, task, description string) {
	_, fields, _ := strings.Cut(row, "\t")
	parts := strings.Split(fields, "\t")
	if len(parts) > 0 {
		dir = parts[0]
	}
	if len(parts) > 1 {
		adaptor = parts[1]
	}
	if len(parts) > 2 {
		task = parts[2]
	}
	if len(parts) > 3 {
		description = parts[3]
	}
	return dir, adaptor, task, description
}

func truncateDisplay(s string, width int) string {
	if displayWidth(s) <= width {
		return s
	}
	if width <= 1 {
		return ""
	}

	var builder strings.Builder
	used := 0
	for _, r := range s {
		runeWidth := displayRuneWidth(r)
		if used+runeWidth > width-1 {
			break
		}
		builder.WriteRune(r)
		used += runeWidth
	}
	return builder.String() + "…"
}

func padDisplay(s string, width int) string {
	s = truncateDisplay(s, width)
	return s + strings.Repeat(" ", max(width-displayWidth(s), 0))
}

func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		width += displayRuneWidth(r)
	}
	return width
}

func displayRuneWidth(r rune) int {
	switch {
	case r == 0 || (r >= 0x0300 && r <= 0x036f):
		return 0
	case r >= 0x1100 && (r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6)):
		return 2
	default:
		return 1
	}
}

func readPickerKey(reader *bufio.Reader) (string, error) {
	r, _, err := reader.ReadRune()
	if err != nil {
		return "", err
	}
	switch r {
	case '\r', '\n':
		return "enter", nil
	case 3:
		return "cancel", nil
	case 8, 127:
		return "backspace", nil
	case 16:
		return "up", nil
	case 14:
		return "down", nil
	case 27:
		var sequence [2]byte
		if _, err := io.ReadFull(reader, sequence[:]); err != nil {
			return "cancel", nil
		}
		if sequence[0] == '[' {
			switch sequence[1] {
			case 'A':
				return "up", nil
			case 'B':
				return "down", nil
			}
		}
		return "cancel", nil
	default:
		return string(r), nil
	}
}
