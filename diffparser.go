// Copyright (c) 2016 AppsCode Inc. <https://github.com/appscode>
// Copyright (c) 2015 Jesse Meek <https://github.com/waigani>
// This program is Free Software see LICENSE file for details.

package diffparser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type FileMode int

const (
	DELETED FileMode = iota
	MODIFIED
	NEW
	MOVE_AWAY
	MOVE_HERE
)

type diffRange struct {

	// starting line number
	Start int

	// the number of lines the change diffHunk applies to
	Length int

	// Each line of the hunk range.
	Lines []*DiffLine
}

type DiffLineMode int

const (
	ADDED DiffLineMode = iota
	REMOVED
	UNCHANGED
)

type DiffLine struct {
	Mode     DiffLineMode
	Number   int
	Content  string
	Position int // the line in the diff
}

type diffHunk struct {
	OrigRange diffRange
	NewRange  diffRange
	ModifyRange diffRange
}

type DiffFile struct {
	Mode     FileMode
	OrigName string
	NewName  string
	Hunks    []*diffHunk
}

type Diff struct {
	Files []*DiffFile
	Raw   string `sql:"type:text"`

	PullID uint `sql:"index"`
}

func (d *Diff) addFile(file *DiffFile) {
	d.Files = append(d.Files, file)
}

// Changed returns a map of filename to lines changed in that file. Deleted
// files are ignored.
func (d *Diff) Changed() map[string][]int {
	dFiles := make(map[string][]int)

	for _, f := range d.Files {
		if f.Mode == DELETED {
			continue
		}

		for _, h := range f.Hunks {
			for _, dl := range h.NewRange.Lines {
				if dl.Mode == ADDED {
					dFiles[f.NewName] = append(dFiles[f.NewName], dl.Number)
				}
			}
		}
	}

	return dFiles
}

func regFind(s string, reg string, group int) string {
	re := regexp.MustCompile(reg)
	return re.FindStringSubmatch(s)[group]
}

func lineMode(line string) (*DiffLineMode, error) {
	var m DiffLineMode
	switch line[:1] {
	case " ":
		m = UNCHANGED
	case "+":
		m = ADDED
	case "-":
		m = REMOVED
	default:
		return nil, fmt.Errorf("could not parse line mode for line: %q", line)
	}
	return &m, nil
}

// Parse takes a diff, such as produced by "git diff", and parses it into a
// Diff struct.
func Parse(diffString string) (*Diff, error) {

	var diff Diff
	diff.Raw = diffString
	lines := strings.Split(diffString, "\n")
	var file *DiffFile
	var hunk *diffHunk
	var ADDEDCount int
	var REMOVEDCount int
	var inHunk bool
	oldFilePrefix := "--- a/"
	newFilePrefix := "+++ b/"
	var hunkLineCount int
	// Parse each line of diff.
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "diff "):
			inHunk = false
			// Start a new file.
			file = &DiffFile{}
			diff.Files = append(diff.Files, file)
			// File mode.
			file.Mode = MODIFIED
		case l == "+++ /dev/null" && inHunk == false :
			file.Mode = DELETED
		case l == "--- /dev/null" && inHunk == false :
			file.Mode = NEW
		case strings.HasPrefix(l, oldFilePrefix) && inHunk == false:
			file.OrigName = strings.TrimPrefix(l, oldFilePrefix)
		case strings.HasPrefix(l, newFilePrefix) && inHunk == false:
			file.NewName = strings.TrimPrefix(l, newFilePrefix)
		case strings.HasPrefix(l, "Binary files") :
			s := strings.Fields(l)
			file.OrigName = strings.TrimPrefix(s[2], "a/")
			if file.OrigName == "/dev/null" {
				file.Mode = NEW
				file.OrigName = ""
			}
			file.NewName = strings.TrimPrefix(s[4], "b/")
			if file.NewName == "/dev/null" {
				file.NewName = ""
				file.Mode = DELETED
			}
		case strings.HasPrefix(l, "rename from"):
			s := strings.Fields(l)
			file.NewName = s[len(s)-1]
			file.OrigName = ""
			file.Mode = MOVE_AWAY
			hunk = &diffHunk{}
			file.Hunks = append(file.Hunks, hunk)
		case strings.HasPrefix(l, "rename to"):
			s := strings.Fields(l)
			name := file.NewName
			file.OrigName = s[len(s)-1]
			// Start a new file.
			file = &DiffFile{}
			file.Mode = MOVE_HERE
			diff.Files = append(diff.Files, file)
			file.NewName = s[len(s)-1]
			file.OrigName = name
			// Start new hunk.
			hunk = &diffHunk{}
			file.Hunks = append(file.Hunks, hunk)
		case strings.HasPrefix(l, "@@ ") && inHunk == false:
			inHunk = true
			// Start new hunk.
			if file.Hunks == nil {
				hunk = &diffHunk{}
				file.Hunks = append(file.Hunks, hunk)
			}
			hunkLineCount = 0
			// Parse hunk heading for ranges
			re := regexp.MustCompile(`@@ \-(\d+),?(\d+)? \+(\d+),?(\d+)? @@`)
			m := re.FindStringSubmatch(l)
			a, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			var b int
			if m[2] == "" {
				b = 0
			} else {
				b, err = strconv.Atoi(m[2])
				if err != nil {
					return nil, err
				}
			}
			c, err := strconv.Atoi(m[3])
			if err != nil {
				return nil, err
			}
			d := c
			if len(m[4]) > 0 {
				d, err = strconv.Atoi(m[4])
				if err != nil {
					return nil, err
				}
			}

			// hunk orig range.
			hunk.OrigRange = diffRange{
				Start:  a,
				Length: b,
			}

			// hunk new range.
			hunk.NewRange = diffRange{
				Start:  c,
				Length: d,
			}

			// (re)set line counts
			ADDEDCount = hunk.NewRange.Start
			REMOVEDCount = hunk.OrigRange.Start
		case inHunk && isSourceLine(l):
			hunkLineCount++
			if l == `\ No newline at end of file` {
				line := DiffLine{
				//Mode:     *m,
				Content:  l,
				//Position: hunkLineCount,
			}
				hunk.ModifyRange.Lines = append(hunk.ModifyRange.Lines, &line)
				break
			}
			m, err := lineMode(l)
			if err != nil {
				return nil, err
			}
			line := DiffLine{
				Mode:     *m,
				Content:  l[1:],
				Position: hunkLineCount,
			}
			newLine := line
			origLine := line

			// add lines to ranges
			switch *m {
			case ADDED:
				newLine.Number = ADDEDCount
				hunk.NewRange.Lines = append(hunk.NewRange.Lines, &newLine)
				ADDEDCount++
				dataDiff := newLine
				dataDiff.Content = "+" + newLine.Content
				hunk.ModifyRange.Lines = append(hunk.ModifyRange.Lines, &dataDiff)

			case REMOVED:
				origLine.Number = REMOVEDCount
				hunk.OrigRange.Lines = append(hunk.OrigRange.Lines, &origLine)
				REMOVEDCount++
				dataDiff := origLine
				dataDiff.Content = "-" + origLine.Content
				hunk.ModifyRange.Lines = append(hunk.ModifyRange.Lines, &dataDiff)

			case UNCHANGED:
				newLine.Number = ADDEDCount
				hunk.NewRange.Lines = append(hunk.NewRange.Lines, &newLine)
				origLine.Number = REMOVEDCount
				hunk.OrigRange.Lines = append(hunk.OrigRange.Lines, &origLine)
				ADDEDCount++
				REMOVEDCount++
				dataDiff := origLine
				dataDiff.Content = " " + origLine.Content
				hunk.ModifyRange.Lines = append(hunk.ModifyRange.Lines, &dataDiff)
			}
		}
	}
	return &diff, nil
}

func isSourceLine(line string) bool {
	if line == `\ No newline at end of file` {
		return true
	}
	if l := len(line); l == 0 {
		return false
	}
	return true
}
