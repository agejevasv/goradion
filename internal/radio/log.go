package radio

import (
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

var log = l.New(&writer{logWriter(), "2006-01-02 15:04:05 "}, "", l.Lshortfile)

func logWriter() io.Writer {
	dir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	w, err := os.Create(path.Join(dir, "goradion.log"))
	if err != nil {
		panic(err)
	}
	return w
}
