package radio

import (
	"io"
	l "log"
	"os"
	"path"
)

var log = l.New(logWriter(), "", 0)

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
