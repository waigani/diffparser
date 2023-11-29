// Copyright (c) 2015 Jesse Meek <https://github.com/waigani>
// This program is Free Software see LICENSE file for details.

package diffparser

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// FileMode represents the file status in a diff
type FileMode int

const (
	// FileModeDeleted if the file is deleted
	FileModeDeleted FileMode = iota
	// FileModeModified if the file is modified
	FileModeModified
	// FileModeNew if the file is created and there is no diff
	FileModeNew
	// FileModeRenamed if the file is renamed
	FileModeRenamed
)

const (
	// DELETED if the file is deleted
	// Deprecated: use FileModeDeleted instead.
	DELETED = FileModeDeleted
	// MODIFIED if the file is modified
	// Deprecated: use FileModeModified instead.
	MODIFIED = FileModeModified
	// NEW if the file is created and there is no diff
	// Deprecated: use FileModeNew instead.
	NEW = FileModeNew
)

// DiffRange contains the DiffLine's
type DiffRange struct {

	// starting line number
	Start int

	// the number of lines the change diffHunk applies to
	Length int

	// Each line of the hunk range.
	Lines []*DiffLine
}

// DiffLineMode tells the line if added, removed or unchanged
type DiffLineMode rune

const (
	// DiffLineModeAdded if the line is added (shown green in diff)
	DiffLineModeAdded DiffLineMode = iota
	// DiffLineModeRemoved if the line is deleted (shown red in diff)
	DiffLineModeRemoved
	// DiffLineModeUnchanged if the line is unchanged (not colored in diff)
	DiffLineModeUnchanged
)

const (
	// ADDED if the line is added (shown green in diff)
	// Deprecated: use DiffLineModeAdded instead.
	ADDED = DiffLineModeAdded
	// REMOVED if the line is deleted (shown red in diff)
	// Deprecated: use DiffLineModeRemoved instead.
	REMOVED = DiffLineModeRemoved
	// UNCHANGED if the line is unchanged (not colored in diff)
	// Deprecated: use DiffLineModeUnchanged instead.
	UNCHANGED = DiffLineModeUnchanged
)

// DiffLine is the least part of an actual diff
type DiffLine struct {
	Mode     DiffLineMode
	Number   int
	Content  string
	Position int // the line in the diff
}

// DiffHunk is a group of difflines
type DiffHunk struct {
	HunkHeader string
	OrigRange  DiffRange
	NewRange   DiffRange
	WholeRange DiffRange
}

// Length returns the hunks line length
func (hunk *DiffHunk) Length() int {
	return len(hunk.WholeRange.Lines) + 1
}

// DiffFile is the sum of diffhunks and holds the changes of the file features
type DiffFile struct {
	DiffHeader string
	Mode       FileMode
	OrigName   string
	NewName    string
	Hunks      []*DiffHunk
	// SimilarityIndex only valid when the mode is FileModeRenamed, ranging from 0 to 100
	SimilarityIndex int
}

// Diff is the collection of DiffFiles
type Diff struct {
	Files []*DiffFile
	Raw   string `sql:"type:text"`

	PullID uint `sql:"index"`
}

// Changed returns a map of filename to lines changed in that file. Deleted
// files are ignored.
func (d *Diff) Changed() map[string][]int {
	dFiles := make(map[string][]int)

	for _, f := range d.Files {
		if f.Mode == FileModeDeleted {
			continue
		}

		for _, h := range f.Hunks {
			for _, dl := range h.NewRange.Lines {
				if dl.Mode == DiffLineModeAdded { // TODO(waigani) return removed
					dFiles[f.NewName] = append(dFiles[f.NewName], dl.Number)
				}
			}
		}
	}

	return dFiles
}

func lineMode(line string) (DiffLineMode, error) {
	switch line[:1] {
	case " ":
		return DiffLineModeUnchanged, nil
	case "+":
		return DiffLineModeAdded, nil
	case "-":
		return DiffLineModeRemoved, nil
	default:
		return DiffLineMode(0), errors.New("could not parse line mode for line: \"" + line + "\"")
	}
}

const (
	oldFilePrefix    = "--- "
	newFilePrefix    = "+++ "
	similarityPrefix = "similarity index "
	renameFromPrefix = "rename from "
	renameToPrefix   = "rename to "
	binaryPrefix     = "Binary files "
)

var (
	reinReg       = regexp.MustCompile(`^index .+$`)
	rempReg       = regexp.MustCompile(`^(-|\+){3} .+$`)
	hunkHeaderReg = regexp.MustCompile(`@@ \-(\d+),?(\d+)? \+(\d+),?(\d+)? @@ ?(.+)?`)
)

// Parse takes a diff, such as produced by "git diff", and parses it into a
// Diff struct.
func Parse(diffString string) (*Diff, error) {
	var (
		diff  = Diff{Raw: diffString}
		lines = strings.Split(diffString, "\n")

		file         *DiffFile
		hunk         *DiffHunk
		addedCount   int
		removedCount int
		inHunk       bool

		diffPosCount    int
		firstHunkInFile bool
	)
	// Parse each line of diff.
	for idx, l := range lines {
		diffPosCount++
		switch {
		case strings.HasPrefix(l, "diff "):
			inHunk = false

			// Start a new file.
			file = &DiffFile{}
			header := l
			if len(lines) > idx+3 {
				index := lines[idx+1]
				if reinReg.MatchString(index) {
					header = header + "\n" + index
				}
				mp1 := lines[idx+2]
				mp2 := lines[idx+3]
				if rempReg.MatchString(mp1) && rempReg.MatchString(mp2) {
					header = header + "\n" + mp1 + "\n" + mp2
				}
			}
			file.DiffHeader = header
			diff.Files = append(diff.Files, file)
			firstHunkInFile = true

			// File mode.
			file.Mode = FileModeModified
		case l == "+++ /dev/null":
			file.Mode = FileModeDeleted
		case l == "--- /dev/null":
			file.Mode = FileModeNew
		case strings.HasPrefix(l, similarityPrefix):
			file.Mode = FileModeRenamed
			file.SimilarityIndex, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(l, similarityPrefix), "%"))
		case strings.HasPrefix(l, oldFilePrefix):
			file.OrigName = parseFileName(strings.TrimPrefix(l, oldFilePrefix))
		case strings.HasPrefix(l, newFilePrefix):
			file.NewName = parseFileName(strings.TrimPrefix(l, newFilePrefix))
		case strings.HasPrefix(l, renameFromPrefix):
			file.OrigName = parseFileName(strings.TrimPrefix(l, renameFromPrefix))
		case strings.HasPrefix(l, renameToPrefix):
			file.NewName = parseFileName(strings.TrimPrefix(l, renameToPrefix))
		case strings.HasPrefix(l, binaryPrefix):
			file.Mode = FileModeModified
			binaryDiffer := strings.TrimSuffix(strings.TrimPrefix(l, binaryPrefix), " differ")
			fileNames := strings.Split(binaryDiffer, " and ")
			if len(fileNames) != 2 {
				return nil, errors.New("invalid binary diff")
			}
			file.OrigName = parseFileName(fileNames[0])
			file.NewName = parseFileName(fileNames[1])
		case strings.HasPrefix(l, "@@ "):
			if firstHunkInFile {
				diffPosCount = 0
				firstHunkInFile = false
			}

			inHunk = true
			// Start new hunk.
			hunk = &DiffHunk{}
			file.Hunks = append(file.Hunks, hunk)

			// Parse hunk heading for ranges
			m := hunkHeaderReg.FindStringSubmatch(l)
			if len(m) < 5 {
				return nil, errors.New("Error parsing line: " + l)
			}
			a, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			b := a
			if len(m[2]) > 0 {
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
			if len(m[5]) > 0 {
				hunk.HunkHeader = m[5]
			}

			// hunk orig range.
			hunk.OrigRange = DiffRange{
				Start:  a,
				Length: b,
			}

			// hunk new range.
			hunk.NewRange = DiffRange{
				Start:  c,
				Length: d,
			}

			// (re)set line counts
			addedCount = hunk.NewRange.Start
			removedCount = hunk.OrigRange.Start
		case inHunk && isSourceLine(l):
			m, err := lineMode(l)
			if err != nil {
				return nil, err
			}
			line := DiffLine{
				Mode:     m,
				Content:  l[1:],
				Position: diffPosCount,
			}
			newLine := line
			origLine := line

			// add lines to ranges
			switch m {
			case DiffLineModeAdded:
				newLine.Number = addedCount
				hunk.NewRange.Lines = append(hunk.NewRange.Lines, &newLine)
				hunk.WholeRange.Lines = append(hunk.WholeRange.Lines, &newLine)
				addedCount++

			case DiffLineModeRemoved:
				origLine.Number = removedCount
				hunk.OrigRange.Lines = append(hunk.OrigRange.Lines, &origLine)
				hunk.WholeRange.Lines = append(hunk.WholeRange.Lines, &origLine)
				removedCount++

			case DiffLineModeUnchanged:
				newLine.Number = addedCount
				hunk.NewRange.Lines = append(hunk.NewRange.Lines, &newLine)
				hunk.WholeRange.Lines = append(hunk.WholeRange.Lines, &newLine)
				origLine.Number = removedCount
				hunk.OrigRange.Lines = append(hunk.OrigRange.Lines, &origLine)
				addedCount++
				removedCount++
			}
		}
	}

	return &diff, nil
}

func parseFileName(filenameWithPrefix string) string {
	if strings.HasPrefix(filenameWithPrefix, "a/") {
		return strings.TrimPrefix(filenameWithPrefix, "a/")
	}
	if strings.HasPrefix(filenameWithPrefix, "b/") {
		return strings.TrimPrefix(filenameWithPrefix, "b/")
	}
	if strings.HasPrefix(filenameWithPrefix, `"`) {
		filenameWithPrefix = strings.TrimSuffix(strings.TrimPrefix(filenameWithPrefix, `"`), `"`)
		if strings.HasPrefix(filenameWithPrefix, "a/") {
			return decodeOctalString(strings.TrimPrefix(filenameWithPrefix, "a/"))
		}
		if strings.HasPrefix(filenameWithPrefix, "b/") {
			return decodeOctalString(strings.TrimPrefix(filenameWithPrefix, "b/"))
		}
		return decodeOctalString(filenameWithPrefix)
	}
	return filenameWithPrefix
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

func decodeOctalString(s string) string {
	s2, err := strconv.Unquote(`"` + s + `"`)
	if err != nil {
		return s
	}
	return s2
}
