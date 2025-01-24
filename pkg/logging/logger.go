package logging

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"
)

const (
	LevelDebug = "DEBUG"
	LevelInfo  = "INFO"
	LevelWarn  = "WARN"
	LevelError = "ERROR"
)

type Logger struct {
	component string
	nodeID    string
}

func NewLogger(component string) *Logger {
	hostname, _ := os.Hostname()
	return &Logger{
		component: component,
		nodeID:    hostname,
	}
}

func (l *Logger) log(level, msg string, args ...interface{}) {
	_, file, line, _ := runtime.Caller(2)
	timestamp := time.Now().Format(time.RFC3339)
	logMsg := fmt.Sprintf(msg, args...)
	log.Printf("%s [%s] [%s] [%s:%d] [%s] %s\n",
		timestamp, level, l.component, file, line, l.nodeID, logMsg)
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.log(LevelDebug, msg, args...)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.log(LevelInfo, msg, args...)
}

func (l *Logger) Warn(msg string, args ...interface{}) {
	l.log(LevelWarn, msg, args...)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.log(LevelError, msg, args...)
}
