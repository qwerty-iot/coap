package coap

import (
	"fmt"
	"log"
	"time"
)

const (
	LogLevelError string = "error"
	LogLevelWarn  string = "warn"
	LogLevelInfo  string = "info"
	LogLevelDebug string = "debug"
)

type LogFunc func(ts time.Time, level string, msg *Message, err error, log string)

var logFunc LogFunc = defaultLogFunc
var logLevel int = 0

func SetLogFunc(lf LogFunc) {
	logFunc = lf
}

func SetLogLevel(level string) {
	switch level {
	case LogLevelError:
		logLevel = 1
	case LogLevelWarn:
		logLevel = 2
	case LogLevelInfo:
		logLevel = 3
	case LogLevelDebug:
		logLevel = 4
	default:
		logLevel = 0
	}
}

func defaultLogFunc(ts time.Time, level string, msg *Message, err error, l string) {
	loc := ""
	if msg != nil && len(msg.Meta.RemoteAddr) != 0 {
		loc = msg.Meta.RemoteAddr
	}
	if err != nil {
		log.Printf(" [" + level + "] [" + loc + "] " + l + "(err: " + err.Error() + ")")
	} else {
		log.Printf(" [" + level + "] [" + loc + "] " + l)
	}
}

func logError(msg *Message, err error, f string, args ...interface{}) {
	if logLevel < 1 {
		return
	}
	logFunc(time.Now(), LogLevelError, msg, err, fmt.Sprintf(f, args...))
}

func logWarn(msg *Message, err error, f string, args ...interface{}) {
	if logLevel < 2 {
		return
	}
	logFunc(time.Now(), LogLevelWarn, msg, err, fmt.Sprintf(f, args...))
}

func logInfo(msg *Message, f string, args ...interface{}) {
	if logLevel < 3 {
		return
	}
	logFunc(time.Now(), LogLevelInfo, msg, nil, fmt.Sprintf(f, args...))
}

func logDebug(msg *Message, f string, args ...interface{}) {
	if logLevel < 4 {
		return
	}
	logFunc(time.Now(), LogLevelDebug, msg, nil, fmt.Sprintf(f, args...))
}
