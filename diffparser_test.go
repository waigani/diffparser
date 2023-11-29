// Copyright (c) 2015 Jesse Meek <https://github.com/waigani>
// This program is Free Software see LICENSE file for details.

package diffparser

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

// TODO(waigani) tests are missing more creative names (spaces, special
// chars), and diffed files that are not in the current directory.

func setup(t *testing.T) *Diff {
	byt, err := ioutil.ReadFile("example.diff")
	require.NoError(t, err)

	diff, err := Parse(string(byt))
	require.NoError(t, err)

	return diff
}

func TestFileModeAndNaming(t *testing.T) {
	diff := setup(t)
	tts := []struct {
		mode     FileMode
		origName string
		newName  string
	}{
		{
			mode:     FileModeModified,
			origName: "file1",
			newName:  "file1",
		},
		{
			mode:     FileModeDeleted,
			origName: "file2",
			newName:  "",
		},
		{
			mode:     FileModeDeleted,
			origName: "file3",
			newName:  "",
		},
		{
			mode:     FileModeNew,
			origName: "",
			newName:  "file4",
		},
		{
			mode:     FileModeNew,
			origName: "",
			newName:  "newname",
		},
		{
			mode:     FileModeDeleted,
			origName: "symlink",
			newName:  "",
		},
		{
			mode:     FileModeModified,
			origName: "file5-中文",
			newName:  "file5-中文",
		},
		{
			mode:     FileModeModified,
			origName: "file6",
			newName:  "file6",
		},
		{
			mode:     FileModeRenamed,
			origName: "file7",
			newName:  "file7-renamed",
		},
		{
			mode:     FileModeRenamed,
			origName: "file8",
			newName:  "file8-中文",
		},
		{
			mode:     FileModeModified,
			origName: "file9.png",
			newName:  "file9.png",
		},
	}
	require.Equal(t, len(diff.Files), len(tts))
	for i, expected := range tts {
		file := diff.Files[i]
		t.Logf("testing file: %v", file)
		require.Equal(t, expected.mode, file.Mode)
		require.Equal(t, expected.origName, file.OrigName)
		require.Equal(t, expected.newName, file.NewName)
	}
}

func TestHunk(t *testing.T) {
	diff := setup(t)
	expectedOrigLines := []DiffLine{
		{
			Mode:     DiffLineModeUnchanged,
			Number:   1,
			Content:  "some",
			Position: 2,
		}, {
			Mode:     DiffLineModeUnchanged,
			Number:   2,
			Content:  "lines",
			Position: 3,
		}, {
			Mode:     DiffLineModeRemoved,
			Number:   3,
			Content:  "in",
			Position: 4,
		}, {
			Mode:     DiffLineModeUnchanged,
			Number:   4,
			Content:  "file1",
			Position: 5,
		},
	}

	expectedNewLines := []DiffLine{
		{
			Mode:     DiffLineModeAdded,
			Number:   1,
			Content:  "add a line",
			Position: 1,
		}, {
			Mode:     DiffLineModeUnchanged,
			Number:   2,
			Content:  "some",
			Position: 2,
		}, {
			Mode:     DiffLineModeUnchanged,
			Number:   3,
			Content:  "lines",
			Position: 3,
		}, {
			Mode:     DiffLineModeUnchanged,
			Number:   4,
			Content:  "file1",
			Position: 5,
		},
	}

	file := diff.Files[0]
	origRange := file.Hunks[0].OrigRange
	newRange := file.Hunks[0].NewRange

	require.Equal(t, 1, origRange.Start)
	require.Equal(t, 4, origRange.Length)
	require.Equal(t, 1, newRange.Start)
	require.Equal(t, 4, newRange.Length)

	for i, line := range expectedOrigLines {
		require.Equal(t, line, *origRange.Lines[i])
	}
	for i, line := range expectedNewLines {
		require.Equal(t, line, *newRange.Lines[i])
	}
}

func TestDecodeOctalString(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{
			input:  `file-1.md`,
			output: "file-1.md",
		},
		{
			input:  `file-\344\270\255\346\226\207.md`,
			output: "file-中文.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.output, decodeOctalString(tt.input))
		})
	}
}
