package main

import (
	"os"

	"github.com/prime-radiant-inc/slackline/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
