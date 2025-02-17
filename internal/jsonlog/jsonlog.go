package jsonlog

import (
	"encoding/json"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

// Level is a type that represents the severity of the log.
type Level int8

// initialize a constant which represent a specific severity level
// we use iota as a shortcut to assign incremental values to the constants
const (
	LevelInfo  Level = iota // has a value of 0
	LevelError              // has a value of 1
	LevelFatal              // has a value of 2
	LevelOff                // has a value of 3
)

// String method to convert the Level type to a string
func (l Level) String() string {
	switch l {
	case LevelInfo:
		return "INFO"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return ""
	}
}

// Logger type to represent the logger. this holds the output destination that the log will be written to,
// the minimum level of severity that logs will be written for, and a mutex to make the logger safe for concurrent use(coordinating the writes)
type Logger struct {
	out      io.Writer
	minLevel Level
	mu       sync.Mutex
}

// New function to create a new Logger instance, which will write logs at or above the specified minimum level to the given output destination
func New(out io.Writer, minLevel Level) *Logger {
	return &Logger{
		out:      out,
		minLevel: minLevel,
	}
}

// PrintInfo method to write an info log entry to the output destination. the log entry will include the log level, specified message and properties
func (l *Logger) PrintInfo(message string, properties map[string]string) {
	l.print(LevelInfo, message, properties)
}

// PrintError method to write an error log entry to the output destination. the log entry will include the specified message and properties
func (l *Logger) PrintError(err error, properties map[string]string) {
	l.print(LevelError, err.Error(), properties)
}

// PrintFatal method to write a fatal log entry to the output destination. the log entry will include the specified message and properties
// after writing the log entry, the application will be terminated by calling os.Exit(1)
func (l *Logger) PrintFatal(err error, properties map[string]string) {
	l.print(LevelFatal, err.Error(), properties)
	os.Exit(1) // for entries at the fatal level, we call os.Exit(1) to terminate the application
}

// we implement the Write method for the Logger type so that it satisfies the io.Writer interface
// this means that we can use a Logger instance as the output destination for the log package's standard library loggers
// this is useful because it allows us to redirect the standard library loggers to our custom logger
func (l *Logger) Write(message []byte) (n int, err error) {
	return l.print(LevelError, string(message), nil)
}

// Print method to write a log entry to the output destination. the log entry will include the specified level, message, and properties
func (l *Logger) print(level Level, message string, properties map[string]string) (int, error) {
	// if the log level is below the minimum level, return without writing anything
	if level < l.minLevel {
		return 0, nil
	}

	// create an anonymous struct to hold the log entry properties
	aux := struct {
		Level      string            `josn:"level"`
		Time       string            `json:"time"`
		Message    string            `json:"message"`
		Properties map[string]string `json:"properties,omitempty"`
		Trace      string            `json:"json,omitempty"`
	}{
		Level:      level.String(),
		Time:       time.Now().UTC().Format(time.RFC3339),
		Message:    message,
		Properties: properties,
	}

	// include the stack trace for logs at the error and fatal levels
	if level >= LevelError {
		aux.Trace = string(debug.Stack())
	}

	// declare a line variable for holding the log entry
	var line []byte

	// marshal the anonymous struct to a JSON and store it in the line variable. if there was a problem
	// creating the JSON, set the contents of the log entry to be that plain text error message
	line, err := json.Marshal(aux)
	if err != nil {
		line = []byte(LevelError.String() + ": unable to marshal log message" + err.Error())
	}

	// lock the logger's mutex to make it safe for concurrent use
	// lock the mutex so that no two writes to the output destination can happen at the same time
	// if we dont do this, it's possible that the text for two or more log entries could be intermingled in the output
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.out.Write(append(line, '\n'))
}
