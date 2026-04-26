package cmd

import (
	"io"

	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:    "create",
	Short:  "(removed — use `slackline provision`)",
	Long:   "The create command has been replaced by `slackline provision bootstrap` and `slackline provision <name>`.",
	Hidden: false,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCreateRemoved(cmd.OutOrStdout(), cmd.OutOrStderr())
	},
}

func runCreateRemoved(stdout, stderr io.Writer) error {
	return &errs.SlackError{
		Code:   errs.Usage,
		Err:    "removed",
		Detail: "slackline create has been split into 'slackline provision bootstrap' (one-time per machine) and 'slackline provision <name>' (per bot). See `slackline provision --help`.",
	}
}
