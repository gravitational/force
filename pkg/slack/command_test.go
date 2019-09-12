package slack

import (
	"gopkg.in/check.v1"
	"testing"

	"github.com/gravitational/trace"
)

// Bootstrap check
func Test(t *testing.T) { check.TestingT(t) }

type BotSuite struct {
}

var _ = check.Suite(&BotSuite{})

func (s *BotSuite) TestSplitWords(c *check.C) {
	type testCase struct {
		input string
		words []string
	}

	testCases := []testCase{
		{
			input: "",
		},
		{
			input: ", ",
		},
		{
			input: "a, b",
			words: []string{"a", "b"},
		},
		{
			input: `"a, b"`,
			words: []string{"a, b"},
		},
		{
			input: `\"a, b"`,
			words: []string{`"a`, `b`},
		},
		{
			input: `\\"a, b"`,
			words: []string{`\`, "a, b"},
		},
		{
			input: `"a, \\"`,
			words: []string{`a, \`},
		},
		{
			input: "build teleport with flags go-build, go-check",
			words: []string{"build", "teleport", "with", "flags", "go-build", "go-check"},
		},
		{
			input: `build teleport with flags "go build", "go\"-check"`,
			words: []string{"build", "teleport", "with", "flags", "go build", `go"-check`},
		},
	}
	for i, tc := range testCases {
		comment := check.Commentf("test case %v %v", i, tc.input)
		out := splitWords(tc.input)
		c.Assert(out, check.DeepEquals, tc.words, comment)
	}
}

func (s *BotSuite) TestParseCommand(c *check.C) {
	type testCase struct {
		name     string
		input    string
		command  Command
		expected map[string]interface{}
		err      error
	}

	testCases := []testCase{
		{
			name:  "simple",
			input: "build teleport with flags go-build, go-check",
			command: Command{
				Name: "build teleport",
				Fields: []Field{
					{
						Name:  "flags",
						Value: StringsEnum{Enum: []string{"go-build", "go-check"}},
					},
				},
			},
			expected: map[string]interface{}{"flags": []string{"go-build", "go-check"}},
		},
		{
			name:  "not recognized",
			input: "build teleport with flags go-build, hello",
			command: Command{
				Name: "build teleport",
				Fields: []Field{
					{
						Name:  "flags",
						Value: StringsEnum{Enum: []string{"go-build", "go-check"}},
					},
				},
			},
			err: trace.BadParameter(""),
		},
		{
			name:  "enum with spaces",
			input: `build teleport with flags "go build", "go check"`,
			command: Command{
				Name: "build teleport",
				Fields: []Field{
					{
						Name:  "flags",
						Value: StringsEnum{Enum: []string{"go build", "go check"}},
					},
				},
			},
			expected: map[string]interface{}{"flags": []string{"go build", "go check"}},
		},
		{
			name:  "couple of parameters",
			input: "build teleport with flags go-build, go-check, with version 3.2.3",
			command: Command{
				Name: "build teleport",
				Fields: []Field{
					{
						Name:  "flags",
						Value: StringsEnum{Enum: []string{"go-build", "go-check"}},
					},
					{
						Name:  "version",
						Value: String{},
					},
				},
			},
			expected: map[string]interface{}{
				"flags":   []string{"go-build", "go-check"},
				"version": "3.2.3",
			},
		},
		{
			name:  "no parameters",
			input: "build teleport",
			command: Command{
				Name: "build teleport",
				Fields: []Field{
					{
						Name:  "flags",
						Value: StringsEnum{Enum: []string{"go-build", "go-check"}},
					},
					{
						Name:  "version",
						Value: String{},
					},
				},
			},
			expected: map[string]interface{}{},
		},
		{
			name:  "not an exact match",
			input: "build teleporta",
			command: Command{
				Name: "build teleport",
			},
			err: trace.NotFound(""),
		},
		{
			name:  "missing required flag",
			input: "build teleport",
			command: Command{
				Name: "build teleport",
				Fields: []Field{
					{
						Name:  "flags",
						Value: StringsEnum{Enum: []string{"go-build", "go-check"}},
					},
					{
						Name:     "version",
						Value:    String{},
						Required: true,
					},
				},
			},
			err: trace.BadParameter(""),
		},
		{
			name:  "expect single value",
			input: "build teleport with version 3.2.3, 4.2.3",
			command: Command{
				Name: "build teleport",
				Fields: []Field{
					{
						Name:  "version",
						Value: String{},
					},
				},
			},
			err: trace.BadParameter(""),
		},
		{
			name:  "unrecognized parameter",
			input: "build teleport with ver 3.2.3",
			command: Command{
				Name: "build teleport",
				Fields: []Field{
					{
						Name:  "version",
						Value: String{},
					},
				},
			},
			err: trace.BadParameter(""),
		},
	}
	for i, tc := range testCases {
		comment := check.Commentf("test case %v %v", i, tc.name)
		out, err := tc.command.Parse(tc.input)
		if tc.err != nil {
			c.Assert(err, check.FitsTypeOf, tc.err, comment)
		} else {
			c.Assert(err, check.IsNil)
			c.Assert(out, check.DeepEquals, tc.expected, comment)
		}
	}
}

func (s *BotSuite) TestConfirm(c *check.C) {
	type testCase struct {
		input string
		fn    func(string) bool
		match bool
	}
	testCases := []testCase{
		{input: "yes", fn: Confirm, match: true},
		{input: "yep", fn: Confirm, match: true},
		{input: "no", fn: Confirm, match: false},
		{input: "cancel", fn: Confirm, match: false},
		{input: "cancel", fn: Abort, match: true},
		{input: "no", fn: Abort, match: true},
	}
	for i, tc := range testCases {
		comment := check.Commentf("expected match %v for input %v in test case %v", tc.match, tc.input, i)
		c.Assert(tc.fn(tc.input), check.Equals, tc.match, comment)
	}
}

func (s *BotSuite) TestHelp(c *check.C) {
	d := Dialog{
		Commands: []Command{{
			Name: "build teleport",
			Fields: []Field{
				{
					Name:  "version",
					Value: String{},
				},
			},
		}},
	}
	p, err := NewParser(d)
	c.Assert(err, check.IsNil)
	chat, err := p.Parse("help")
	c.Assert(err, check.IsNil)
	c.Assert(chat.RequestedHelp, check.Equals, true)

	chat, err = p.Parse("help how to build teleport")
	c.Assert(err, check.IsNil)
	c.Assert(chat.RequestedHelp, check.Equals, true)
	c.Assert(chat.Command.Name, check.Equals, "build teleport")
}
