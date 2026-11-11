package main

import (
	"os"

	"github.com/karolsko/gobunshin/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}))
}
