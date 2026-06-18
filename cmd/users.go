package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	usersMatch  string
	usersFormat string
)

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "List or search workspace users (resolve a name/handle to a user ID)",
	Long:  "List workspace users, or filter with --match to resolve a handle, display name, real name, or ID to a user ID for mentions.",
	RunE: func(cmd *cobra.Command, args []string) error {
		api, err := loadBotAPI()
		if err != nil {
			return err
		}
		outputFormat, err := parseOutputFormat(usersFormat)
		if err != nil {
			return err
		}
		all, err := slackpkg.NewUserDirectory(api).All()
		if err != nil {
			if isAuthError(err) {
				return errs.AuthError(err.Error())
			}
			return &errs.SlackError{Code: errs.SlackAPI, Err: "users_failed", Detail: err.Error()}
		}
		return writeUsersOutput(cmd.OutOrStdout(), outputFormat, filterUsers(all, usersMatch))
	},
}

// filterUsers drops deactivated users and, when match is non-empty, keeps only
// users whose ID, handle, display name, or real name contains match
// (case-insensitive).
func filterUsers(users []goslack.User, match string) []goslack.User {
	match = strings.ToLower(strings.TrimSpace(match))
	var out []goslack.User
	for _, u := range users {
		if u.Deleted {
			continue
		}
		if match != "" && !userMatches(u, match) {
			continue
		}
		out = append(out, u)
	}
	return out
}

func userMatches(u goslack.User, match string) bool {
	for _, field := range []string{u.ID, u.Name, u.Profile.DisplayName, u.RealName} {
		if strings.Contains(strings.ToLower(field), match) {
			return true
		}
	}
	return false
}

type userJSON struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name,omitempty"`
	RealName    string `json:"real_name,omitempty"`
}

func writeUsersOutput(w io.Writer, outputFormat string, users []goslack.User) error {
	if outputFormat == outputFormatJSON {
		return writeUsersJSON(w, users)
	}
	return writeUsersTable(w, users)
}

func writeUsersJSON(w io.Writer, users []goslack.User) error {
	out := make([]userJSON, len(users))
	for i, u := range users {
		out[i] = userJSON{
			ID:          u.ID,
			Handle:      u.Name,
			DisplayName: u.Profile.DisplayName,
			RealName:    u.RealName,
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

func writeUsersTable(out io.Writer, users []goslack.User) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tHANDLE\tDISPLAY\tREAL NAME")
	for _, u := range users {
		_, _ = fmt.Fprintf(w, "%s\t@%s\t%s\t%s\n", u.ID, u.Name, u.Profile.DisplayName, u.RealName)
	}
	return w.Flush()
}

func init() {
	usersCmd.Flags().StringVar(&usersMatch, "match", "", "filter by handle, display name, real name, or ID (case-insensitive substring)")
	usersCmd.Flags().StringVar(&usersFormat, "format", outputFormatText, "output format: text or json")
	rootCmd.AddCommand(usersCmd)
}
