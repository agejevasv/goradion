package radio

import (
	"fmt"
	"io"
	l "log"
	"os"
	"time"
)

type writer struct {
	io.Writer
	timeFormat string
}

var log *l.Logger

func InitLog(enabled bool) {
	var w io.Writer

	if enabled {
		w = logWriter()
	} else {
		w = io.Discard
	}

	log = l.New(&writer{w, time.DateTime}, fmt.Sprintf(" %d ", os.Getpid()), l.Lshortfile)
}

func (w writer) Write(b []byte) (n int, err error) {
	return w.Writer.Write(append([]byte(time.Now().Format(w.timeFormat)), b...))
}

func logWriter() io.Writer {
	w, err := os.OpenFile("goradion.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		panic(err)
	}

	return w
}
