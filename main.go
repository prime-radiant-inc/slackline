package main

import (
	"os"

	"github.com/prime-radiant-inc/slackline/cmd"
)

var version = "dev"

func main() {
	cmd.SetVersion(version)
	os.Exit(cmd.Execute())
}
