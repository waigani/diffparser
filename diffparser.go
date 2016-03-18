// Copyright (c) 2015 Jesse Meek <https://github.com/waigani>
// This program is Free Software see LICENSE file for details.

package diffparser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/errors"
)

type FileMode int

const (
	DELETED FileMode = iota
	MODIFIED
	NEW
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
}

type DiffFile struct {
	Mode      FileMode
	OrigName  string
	NewName   string
	Additions int
	Deletions int
	Hunks     []*diffHunk
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
		return nil, errors.Errorf("could not parse line mode for line: %q", line)
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
		case l == "+++ /dev/null":
			file.Mode = DELETED
		case l == "--- /dev/null":
			file.Mode = NEW
		case strings.HasPrefix(l, oldFilePrefix):
			file.OrigName = strings.TrimPrefix(l, oldFilePrefix)
		case strings.HasPrefix(l, newFilePrefix):
			file.NewName = strings.TrimPrefix(l, newFilePrefix)
		case strings.HasPrefix(l, "@@ "):
			inHunk = true
			// Start new hunk.
			hunk = &diffHunk{}
			file.Hunks = append(file.Hunks, hunk)
			hunkLineCount = 0
			// Parse hunk heading for ranges
			re := regexp.MustCompile(`@@ \-(\d+),(\d+) \+(\d+),?(\d+)? @@`)
			m := re.FindStringSubmatch(l)
			a, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			b, err := strconv.Atoi(m[2])
			if err != nil {
				return nil, err
			}
			c, err := strconv.Atoi(m[3])
			if err != nil {
				return nil, errors.Trace(err)
			}
			d := c
			if len(m[4]) > 0 {
				d, err = strconv.Atoi(m[4])
				if err != nil {
					return nil, errors.Trace(err)
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
			m, err := lineMode(l)
			if err != nil {
				return nil, errors.Trace(err)
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
				file.Additions++

			case REMOVED:
				origLine.Number = REMOVEDCount
				hunk.OrigRange.Lines = append(hunk.OrigRange.Lines, &origLine)
				REMOVEDCount++
				file.Deletions++

			case UNCHANGED:
				newLine.Number = ADDEDCount
				hunk.NewRange.Lines = append(hunk.NewRange.Lines, &newLine)
				origLine.Number = REMOVEDCount
				hunk.OrigRange.Lines = append(hunk.OrigRange.Lines, &origLine)
				ADDEDCount++
				REMOVEDCount++
			}
		}
	}

	return &diff, nil
}

func isSourceLine(line string) bool {
	if line == `\ No newline at end of file` {
		return false
	}
	if l := len(line); l == 0 || (l >= 3 && (line[:3] == "---" || line[:3] == "+++")) {
		return false
	}
	return true
}
