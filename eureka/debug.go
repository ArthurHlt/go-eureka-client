package eureka

import (
	"fmt"
	"log"
	"os"
	"strings"
)

var logger *eurekaLogger

func SetLogger(l *log.Logger) {
	logger = &eurekaLogger{l}
}

func GetLogger() *log.Logger {
	return logger.log
}

type eurekaLogger struct {
	log *log.Logger
}

func (p *eurekaLogger) Debug(args ...interface{}) {
	msg := "DEBUG: " + fmt.Sprint(args...)
	p.log.Println(msg)
}

func (p *eurekaLogger) Debugf(f string, args ...interface{}) {
	msg := "DEBUG: " + fmt.Sprintf(f, args...)
	// Append newline if necessary
	if !strings.HasSuffix(msg, "\n") {
		msg = msg + "\n"
	}
	p.log.Print(msg)
}

func (p *eurekaLogger) Warning(args ...interface{}) {
	msg := "WARNING: " + fmt.Sprint(args...)
	p.log.Println(msg)
}

func (p *eurekaLogger) Warningf(f string, args ...interface{}) {
	msg := "WARNING: " + fmt.Sprintf(f, args...)
	// Append newline if necessary
	if !strings.HasSuffix(msg, "\n") {
		msg = msg + "\n"
	}
	p.log.Print(msg)
}

func init() {
	// Default logger uses the go default log.
	SetLogger(log.New(os.Stdout, "go-eureka", log.LstdFlags))
}
