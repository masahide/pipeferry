package buildinfo

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func String() string {
	return fmt.Sprintf("pipeferry %s (commit=%s, built=%s, %s/%s)", Version, Commit, BuildDate, runtime.GOOS, runtime.GOARCH)
}
