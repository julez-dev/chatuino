package main

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/urfave/cli/v2"
)

var (
	Version = "dev"
	Commit  = "empty-commit"
	Date    = "empty-date"
)

var versionCMD = &cli.Command{
	Name:    "version",
	Aliases: []string{"v"},
	Usage:   "Print the version",
	Action: func(_ *cli.Context) error {
		res := fmt.Sprintf("Chatuino version %s\n"+
			"commit: %s\n"+
			"built at: %s\n"+
			"goos: %s\n"+
			"goarch: %s\n"+
			"go version: %s\n",
			Version, Commit, Date, runtime.GOOS, runtime.GOARCH, runtime.Version(),
		)

		if _, err := io.WriteString(os.Stdout, res); err != nil {
			return err
		}

		return nil
	},
}
