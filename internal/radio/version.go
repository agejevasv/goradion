package radio

import (
	"fmt"
	"runtime"
)

const Version = "0.6.1"

func VersionString() string {
	return fmt.Sprintf("goradion v%s (%s/%s)", Version, runtime.GOARCH, runtime.GOOS)
}
