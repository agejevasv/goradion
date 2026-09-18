// Package logging is goradion's debug log, off unless enabled with -d.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync/atomic"
	"time"
)

const fileName = "goradion.log"

var logger atomic.Pointer[log.Logger]

func init() {
	logger.Store(log.New(io.Discard, "", 0))
}

// Init enables the log in goradion.log in the current directory.
func Init(enabled bool) error {
	if !enabled {
		logger.Store(log.New(io.Discard, "", 0))
		return nil
	}
	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	logger.Store(log.New(timestamped{f}, fmt.Sprintf(" %d ", os.Getpid()), log.Lshortfile))
	return nil
}

// timestamped puts the time before the prefix, which log.Logger can't.
type timestamped struct{ io.Writer }

func (w timestamped) Write(b []byte) (int, error) {
	return w.Writer.Write(append([]byte(time.Now().Format(time.DateTime)), b...))
}

func Printf(format string, v ...any) {
	logger.Load().Output(2, fmt.Sprintf(format, v...))
}

func Println(v ...any) {
	logger.Load().Output(2, fmt.Sprintln(v...))
}
