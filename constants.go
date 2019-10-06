package force

const (
	// HumanDateFormat is a user friendly date format
	HumanDateFormat = "Jan _2 15:04 UTC"
)

// ContextKey is a special type used to set force-related
// context value, is recommended by context package to use
// separate type for context values to prevent
// namespace clash
type ContextKey string

const (
	// KeyCurrentDir is a current directory
	KeyCurrentDir = ContextKey("current.dir")
	// KeyError is an error value
	KeyError = ContextKey("error")
	// KeyLog is a logger associated with this execution
	KeyLog = ContextKey("log")
	// KeyProc is a process name
	KeyProc = "proc"
	// KeyID is a unique identifier of the run
	KeyID = "id"
	// KeyForce is a name of the force CI
	KeyForce = "force"
	// KeyEvent is an event produced by watchers
	KeyEvent = "event"
	// Underscore is underscore symbol
	Underscore = "_"
	StringType = "string"
	IntType    = "int"
	BoolType   = "bool"
)
