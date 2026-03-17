package main

import (
	"os"

	"github.com/prime-radiant/slackline/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
