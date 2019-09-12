package slack

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gravitational/trace"
)

// Command is matched by regular expression
type Command struct {
	// Name is a unique command name
	Name string
	// Fields are command fields
	Fields []Field
	// Help is a help message
	Help string
	// Confirm requests confirmation for command
	Confirm bool
}

// Field is a command field
type Field struct {
	// Name is a field name
	Name string
	// Required sets whether the field is required
	Required bool
	// Help sets the help
	Help string
	// Value is a field parser
	Value FieldParser
}

func (f *Field) HelpMessage() string {
	if f.Help != "" {
		return f.Help
	}
	return fmt.Sprintf("`%v` %v", f.Name, f.Value.HelpMessage())
}

// FieldParser parses command
type FieldParser interface {
	// Parse parses command
	Parse(words []string) (interface{}, error)
	// HelpMessage
	HelpMessage() string
	// DefaultValue returns default value
	DefaultValue() interface{}
}

// StringsEnum enumerates strings
type StringsEnum struct {
	Enum       []string
	Default    []string
	DefaultAll bool
}

// DefaultValue returns default value
func (p StringsEnum) DefaultValue() interface{} {
	if len(p.Default) > 0 {
		return p.Default
	}
	if p.DefaultAll {
		return p.Enum
	}
	return []string{""}
}

// SupportedValues lists supported values
func (p StringsEnum) SupportedValues() string {
	out := make([]string, len(p.Enum))
	for i, v := range p.Enum {
		out[i] = fmt.Sprintf("`%v`", v)
	}
	return strings.Join(out, ", ")
}

// HelpMessage rr
func (p StringsEnum) HelpMessage() string {
	return fmt.Sprintf("one of %v", strings.Join(p.Enum, ","))
}

// Parse parses the command
func (p StringsEnum) Parse(words []string) (interface{}, error) {
	m := make(map[string]struct{})
	for _, enum := range p.Enum {
		m[enum] = struct{}{}
	}
	for _, w := range words {
		if _, ok := m[w]; !ok {
			return "", trace.BadParameter(
				"parameter `%v` is not recognized, supported values are %v", w, p.SupportedValues())
		}
	}
	return words, nil
}

type String struct {
	Default string
}

func (s String) HelpMessage() string {
	return "string"
}

func (s String) Parse(input []string) (interface{}, error) {
	if len(input) != 1 {
		return "", trace.BadParameter("expected a single value, got %v", input)
	}
	return input[0], nil
}

// Default returns default value
func (s String) DefaultValue() interface{} {
	return s.Default
}

// Confirm parses input looking for confirmation message
func Confirm(input string) bool {
	return reConfirm.MatchString(strings.ToLower(input))
}

// Abort parses stop cancel message
func Abort(input string) bool {
	return reAbort.MatchString(strings.ToLower(input))
}

// ConfirmationMessage returns confirmation message
func (c Command) ConfirmationMessage(values map[string]interface{}) string {
	parameters := make([]string, 0, len(values))
	for key, val := range values {
		parameters = append(parameters, fmt.Sprintf("%v: %v", key, val))
	}
	withParameters := ""
	if len(parameters) != 0 {
		withParameters = fmt.Sprintf(" with parameters `%v`", strings.Join(parameters, ", "))
	}
	return fmt.Sprintf("Please confirm that you want to execute command `%v`%v (yes/no).", c.Name, withParameters)
}

// HelpMessage returns command help messsage
func (c Command) HelpMessage() string {
	if c.Help != "" {
		return fmt.Sprintf("`%v` %v", c.Name, c.Help)
	}
	fields := []string{}
	for _, f := range c.Fields {
		fields = append(fields, f.HelpMessage())
	}
	return fmt.Sprintf("`%v`, parameters: %v", c.Name, strings.Join(fields, ","))
}

// CheckAndSetDefaults checks and sets defaults for command
func (c Command) CheckAndSetDefaults() error {
	if c.Name == "" {
		return trace.BadParameter("supply Command{Name: }")
	}
	return nil
}

// Parse will return not found if could not parse,
// error otherwise
func (c Command) Parse(input string) (map[string]interface{}, error) {
	// now split words into groups
	fields := make(map[string]Field)
	fieldNames := make([]string, len(c.Fields))
	for i := range c.Fields {
		field := c.Fields[i]
		fieldNames[i] = "`" + field.Name + "`"
		if field.Name == "" {
			return nil, trace.BadParameter(`supply Field{Name: ""}`)
		}
		if _, ok := fields[field.Name]; ok {
			return nil, trace.BadParameter("field %v is already defined", field.Name)
		}
		fields[field.Name] = field
	}
	// at first, try matching command name
	expr := `^\s*` + regexp.QuoteMeta(c.Name) + `(\s.*)?$`
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	matches := re.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return nil, trace.NotFound("expression %v did not match input %v", expr, input)
	}
	if len(matches) > 1 {
		return nil, trace.NotFound("expression %v matched multiple times", expr)
	}
	match := matches[0]
	// parse the rest of the string
	rest := match[1]
	//
	// parse simple grammar:
	//
	// argument-list -> argument | argument argument-list
	// argument -> <with> <field-name> <values>
	// CSV helps to parse quoted strings properly
	words := splitWords(rest)
	processed := make(map[string]interface{})
	for {
		if len(words) == 0 {
			break
		}
		if len(words) < 2 {
			return nil, trace.BadParameter("for command `%v` parameters, expected `with` followed by parameter names", c.Name)
		}
		if !isDelim(words[0]) {
			return nil, trace.BadParameter(
				"for command `%v` parameters, expected `with` followed by the field name, for example `with version 2.2.3`", c.Name)
		}
		fieldName := words[1]
		field, ok := fields[fieldName]
		if !ok {
			return nil, trace.BadParameter(
				"could not recognize parameter `%v`, supported parameters are %v",
				fieldName,
				strings.Join(fieldNames, ", "))
		}
		args := words[2:]
		rest := []string{}
		for i, word := range args {
			if isDelim(word) {
				rest = args[i:]
				args = args[0:i]
				break
			}
		}
		if len(args) == 0 {
			continue
		}
		out, err := field.Value.Parse(args)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		processed[fieldName] = out
		words = rest
	}
	for fieldName, field := range fields {
		if _, found := processed[fieldName]; !found {
			if field.Required {
				return nil, trace.BadParameter("provide required parameter `%v`, for example `%v with %v <value>`",
					fieldName, c.Name, fieldName)
			}
		}
	}
	return processed, nil
}

func splitWords(input string) []string {
	var words []string
	var word []rune
	var inString bool
	var prevEscape bool // previous rune was escape
	for _, char := range input {
		switch char {
		case '\\':
			if prevEscape {
				word = append(word, '\\')
				prevEscape = false
			} else {
				prevEscape = true
			}
		case '"':
			// escaped string, treat as a part of word
			if prevEscape {
				word = append(word, char)
				prevEscape = false
			} else {
				// end of string
				if inString {
					words = append(words, string(word))
					word = []rune{}
					inString = false
				} else {
					// start of the string
					inString = true
					if len(word) != 0 {
						words = append(words, string(word))
					}
					word = []rune{}
				}
			}
		case ',', ' ', '\t':
			if inString {
				word = append(word, char)
				prevEscape = false
			} else {
				if prevEscape {
					prevEscape = false
					word = append(word, char)
				} else {
					if len(word) != 0 {
						words = append(words, string(word))
						word = []rune{}
					}
				}
			}
		default:
			word = append(word, char)
		}
	}
	if len(word) != 0 {
		words = append(words, string(word))
	}
	return words
}

func isDelim(w string) bool {
	switch strings.ToLower(w) {
	case "with", "and":
		return true
	}
	return false
}

// Chat is a result of parsed chat command
type Chat struct {
	// Command is a command that has matched
	Command Command
	// Values is a list of parsed values
	Values map[string]interface{}
	// RequestedHelp is set when user has requested help,
	// in this case Command contains the command the user has requested
	// help about
	RequestedHelp bool
}

// SupportedCommands returns a list of supported commands
func (p *ChatParser) SupportedCommands() []string {
	commands := make([]string, 0, len(p.dialog.Commands)+2)
	commands = append(commands, helpCommand)
	commands = append(commands, helpCommand+" <command name>")
	for i := range p.dialog.Commands {
		command := p.dialog.Commands[i]
		commands = append(commands, command.Name)
	}
	return commands
}

// HelpMessage generates and returns help message for individual command
// or generic message if commandName is empty
func (p *ChatParser) HelpMessage(commandName string) string {
	if commandName != "" {
		for _, cmd := range p.dialog.Commands {
			if cmd.Name == commandName {
				return cmd.HelpMessage()
			}
		}
	}
	commands := []string{
		"* `help` to print this help message",
		"* `help <command>` to get help on individual command",
	}
	for _, cmd := range p.dialog.Commands {
		commands = append(commands, fmt.Sprintf("* %v", cmd.HelpMessage()))
	}
	return fmt.Sprintf(helpMessageTemplate, strings.Join(commands, "\n"))
}

// Parse parses user input into structured chat
func (p *ChatParser) Parse(input string) (*Chat, error) {
	match := reHelp.FindStringSubmatch(input)
	if len(match) != 0 {
		chat := &Chat{
			RequestedHelp: true,
		}
		rest := strings.TrimSpace(match[1])
		if rest == "" {
			return chat, nil
		}
		// try to locate commnand by building a regexp that will match
		// any of the subcommand names
		commands := []string{}
		for i := range p.dialog.Commands {
			command := p.dialog.Commands[i]
			commands = append(commands, "("+regexp.QuoteMeta(command.Name)+")")
		}
		expr := strings.Join(commands, "|")
		reCommand, err := regexp.Compile(expr)
		if err != nil {
			return nil, trace.BadParameter("could not parse constructed regex %v: %v", expr, err)
		}
		match := reCommand.FindStringSubmatch(rest)
		if len(match) == 0 {
			return chat, nil
		}
		for i, val := range match {
			// 0 index is the expression itself
			if i != 0 && val != "" {
				if i-1 < len(p.dialog.Commands) {
					chat.Command = p.dialog.Commands[i-1]
					break
				}
			}
		}
		return chat, nil
	}
	commands := []string{helpCommand}
	for i := range p.dialog.Commands {
		command := p.dialog.Commands[i]
		commands = append(commands, command.Name)
		out, err := command.Parse(input)
		if err == nil {
			return &Chat{
				Command: command,
				Values:  out,
			}, nil
		}
		if !trace.IsNotFound(err) {
			return nil, trace.Wrap(err)
		}
	}
	return nil, trace.NotFound("could not parse the command")
}

// Dialog consists of a list of commands
type Dialog struct {
	Commands []Command
}

// ChatParser parses chat based on the dialog
type ChatParser struct {
	dialog Dialog
}

// NewParser returns new chat parser based on the dialog
// help commands are generated programmatically
func NewParser(d Dialog) (*ChatParser, error) {
	for i := range d.Commands {
		cmd := d.Commands[i]
		if err := cmd.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
	}
	return &ChatParser{
		dialog: d,
	}, nil
}

const (
	helpCommand              = "help"
	helpMessageTemplate      = "Here are the supported commands:\n%v\n"
	unrecognizedConvoMessage = "I'm sorry, I did not recognize this conversation. You can type `help` to get a help blurb."
)

var (
	reHelp    = regexp.MustCompile(`^\s*help(\s.*)?`)
	reConfirm = regexp.MustCompile(`yes|yep|yeah|yep|confirm|ok|confirmed`)
	reAbort   = regexp.MustCompile(`no|not|abort|cancel`)
)
