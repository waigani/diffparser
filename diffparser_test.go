// Copyright (c) 2015 Jesse Meek <https://github.com/waigani>
// This program is Free Software see LICENSE file for details.

package diffparser_test

import (
	"io/ioutil"
	"testing"

	jt "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/waigani/diffparser"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type suite struct {
	jt.CleanupSuite
	rawdiff string
	diff    *diffparser.Diff
}

var _ = gc.Suite(&suite{})

func (s *suite) SetUpSuite(c *gc.C) {
	byt, err := ioutil.ReadFile("example.diff")
	c.Assert(err, jc.ErrorIsNil)
	s.rawdiff = string(byt)
}

// TODO(waigani) tests are missing more creative names (spaces, special
// chars), and diffed files that are not in the current directory.

func (s *suite) TestFileModeAndNaming(c *gc.C) {
	diff, err := diffparser.Parse(s.rawdiff)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(diff.Files, gc.HasLen, 5)

	for i, expected := range []struct {
		mode     diffparser.FileMode
		origName string
		newName  string
	}{
		{
			mode:     diffparser.MODIFIED,
			origName: "file1",
			newName:  "file1",
		},
		{
			mode:     diffparser.DELETED,
			origName: "file2",
			newName:  "",
		},
		{
			mode:     diffparser.DELETED,
			origName: "file3",
			newName:  "",
		},
		{
			mode:     diffparser.NEW,
			origName: "",
			newName:  "file4",
		},
		{
			mode:     diffparser.NEW,
			origName: "",
			newName:  "newname",
		},
	} {
		file := diff.Files[i]
		c.Logf("testing file: %v", file)
		c.Assert(file.Mode, gc.Equals, expected.mode)
		c.Assert(file.OrigName, gc.Equals, expected.origName)
		c.Assert(file.NewName, gc.Equals, expected.newName)
	}
}

func (s *suite) TestHunk(c *gc.C) {
	diff, err := diffparser.Parse(s.rawdiff)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(diff.Files, gc.HasLen, 5)

	expectedOrigLines := []diffparser.DiffLine{
		{
			Mode:     diffparser.UNCHANGED,
			Number:   1,
			Content:  "some",
			Position: 2,
		}, {
			Mode:     diffparser.UNCHANGED,
			Number:   2,
			Content:  "lines",
			Position: 3,
		}, {
			Mode:     diffparser.REMOVED,
			Number:   3,
			Content:  "in",
			Position: 4,
		}, {
			Mode:     diffparser.UNCHANGED,
			Number:   4,
			Content:  "file1",
			Position: 5,
		},
	}

	expectedNewLines := []diffparser.DiffLine{
		{
			Mode:     diffparser.ADDED,
			Number:   1,
			Content:  "add a line",
			Position: 1,
		}, {
			Mode:     diffparser.UNCHANGED,
			Number:   2,
			Content:  "some",
			Position: 2,
		}, {
			Mode:     diffparser.UNCHANGED,
			Number:   3,
			Content:  "lines",
			Position: 3,
		}, {
			Mode:     diffparser.UNCHANGED,
			Number:   4,
			Content:  "file1",
			Position: 5,
		},
	}

	file := diff.Files[0]
	origRange := file.Hunks[0].OrigRange
	newRange := file.Hunks[0].NewRange

	c.Assert(origRange.Start, gc.Equals, 1)
	c.Assert(origRange.Length, gc.Equals, 4)
	c.Assert(newRange.Start, gc.Equals, 1)
	c.Assert(newRange.Length, gc.Equals, 4)

	for i, line := range expectedOrigLines {
		c.Assert(*origRange.Lines[i], gc.DeepEquals, line)
	}
	for i, line := range expectedNewLines {
		c.Assert(*newRange.Lines[i], gc.DeepEquals, line)
	}
}
