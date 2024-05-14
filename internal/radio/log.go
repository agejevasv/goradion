package radio

import (
	"fmt"
	"io"
	l "log"
	"os"
	"path"
	"time"
)

type writer struct {
	io.Writer
	timeFormat string
}

func (w writer) Write(b []byte) (n int, err error) {
	return w.Writer.Write(append([]byte(time.Now().Format(w.timeFormat)), b...))
}

var log = l.New(
	&writer{logWriter(), "2006-01-02 15:04:05 "},
	fmt.Sprintf("[%d]", os.Getpid()),
	l.Lshortfile)

func logWriter() io.Writer {
	dir, err := os.UserHomeDir()

	if err != nil {
		panic(err)
	}

	w, err := os.OpenFile(
		path.Join(dir, "goradion.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644)

	if err != nil {
		panic(err)
	}

	return w
}
