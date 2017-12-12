package runner

// This file contains the implementation of a logger that adorns the logxi package with
// some common information not normally supplied by the generic code

import (
	"os"

	"github.com/mgutz/logxi"
)

var (
	hostName string
)

func init() {
	hostName, _ = os.Hostname()
}

type Logger struct {
	log logxi.Logger
}

func NewLogger(component string) (log *Logger) {
	return &Logger{
		log: logxi.New(component),
	}
}

func (l *Logger) Trace(msg string, args ...interface{}) {
	allArgs := append([]interface{}{}, args...)
	allArgs = append(allArgs, "host")
	allArgs = append(allArgs, hostName)
	l.log.Trace(msg, allArgs)
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	allArgs := append([]interface{}{}, args...)
	allArgs = append(allArgs, "host")
	allArgs = append(allArgs, hostName)
	l.log.Debug(msg, allArgs)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	allArgs := append([]interface{}{}, args...)
	allArgs = append(allArgs, "host")
	allArgs = append(allArgs, hostName)
	l.log.Info(msg, allArgs)
}

func (l *Logger) Warn(msg string, args ...interface{}) error {
	allArgs := append([]interface{}{}, args...)
	allArgs = append(allArgs, "host")
	allArgs = append(allArgs, hostName)
	return l.log.Warn(msg, allArgs)
}

func (l *Logger) Error(msg string, args ...interface{}) error {
	allArgs := append([]interface{}{}, args...)
	allArgs = append(allArgs, "host")
	allArgs = append(allArgs, hostName)
	return l.log.Error(msg, allArgs)
}

func (l *Logger) Fatal(msg string, args ...interface{}) {
	allArgs := append([]interface{}{}, args...)
	allArgs = append(allArgs, "host")
	allArgs = append(allArgs, hostName)
	l.log.Fatal(msg, allArgs)
}

func (l *Logger) Log(level int, msg string, args []interface{}) {
	allArgs := append([]interface{}{}, args...)
	allArgs = append(allArgs, "host")
	allArgs = append(allArgs, hostName)
	l.log.Log(level, msg, allArgs)
}

func (l *Logger) SetLevel(lvl int) {
	l.log.SetLevel(lvl)
}

func (l *Logger) IsTrace() bool {
	return l.log.IsTrace()
}

func (l *Logger) IsDebug() bool {
	return l.log.IsDebug()
}

func (l *Logger) IsInfo() bool {
	return l.log.IsInfo()
}

func (l *Logger) IsWarn() bool {
	return l.log.IsWarn()
}
