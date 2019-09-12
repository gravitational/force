package force

import (
	"fmt"
	"go/token"
	"strings"
	"unicode"
)

// CodeError wraps error in the code
type CodeError struct {
	Snippet Snippet
	Err     error
}

// Error returns user friendly error
func (e *CodeError) Error() string {
	return Capitalize(e.Err.Error()) + "\n" + e.Snippet.String() + "\n"
}

// Snippet is a snippet captured from the source file based on the position
type Snippet struct {
	Pos    token.Position
	Text   string
	Offset int
}

// String returns user friendly representation of the offset
func (s Snippet) String() string {
	text := strings.ReplaceAll(s.Text, "\t", " ")
	pointer := strings.Repeat(" ", s.Offset) + "^"
	location := fmt.Sprintf("---- file %v, line %v, column %v ----", s.Pos.Filename, s.Pos.Line, s.Pos.Column)
	delim := strings.Repeat("-", len(location))
	return strings.Join([]string{"", delim, text, pointer, location, ""}, "\n")
}

// CaptureSnippet captures snippet near position near newline
func CaptureSnippet(pos token.Position, text string) Snippet {
	if pos.Offset >= len(text) {
		return Snippet{Pos: pos}
	}
	// go back until the start of the previous line
	start := pos.Offset - 1
	if start < 0 {
		start = 0
	}
	for ; start >= 0; start-- {
		if text[start] == '\n' {
			start++
			if start > len(text) {
				start = len(text) - 1
			}
			break
		}
	}
	var snippet []rune
	// go forward until the end of the line
	for i, r := range text[start:] {
		if i > 256 || r == '\n' {
			break
		}
		snippet = append(snippet, r)
	}
	return Snippet{
		Pos:    pos,
		Text:   string(snippet),
		Offset: pos.Offset - start,
	}
}

// Capitalize returns a copy of the string
// with first rune converted to capital letter
func Capitalize(s string) string {
	// Use a closure here to remember state.
	// Hackish but effective. Depends on Map scanning in order and calling
	// the closure once per rune.
	done := false
	return strings.Map(
		func(r rune) rune {
			if done {
				return r
			}
			done = true
			return unicode.ToTitle(r)
		},
		s)
}
