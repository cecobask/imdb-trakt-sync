package root

import (
	"os"

	"github.com/cecobask/imdb-trakt-sync/cmd/sync"
	"github.com/spf13/cobra"
)

const (
	commandNameRoot  = "its"
	commandAliasRoot = "imdb-trakt-sync"
)

func NewRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:     commandNameRoot,
		Aliases: []string{commandAliasRoot},
		Short:   "imdb-trakt-sync command line interface",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.SetOut(os.Stdout)
			cmd.SetErr(os.Stderr)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		SilenceUsage: true,
	}
	command.SetHelpCommand(&cobra.Command{
		Hidden: true,
	})
	command.AddCommand(
		sync.NewSyncCommand(),
	)
	command.SetOut(os.Stdout)
	command.SetErr(os.Stderr)
	return command
}
