package cmd

import (
	"os"

	"github.com/cecobask/imdb-trakt-sync/cmd/sync"
	"github.com/spf13/cobra"
)

const CommandNameRoot = "its"

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     CommandNameRoot,
		Aliases: []string{"imdb-trakt-sync"},
		Short:   "imdb-trakt-sync command line interface",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		SilenceUsage: true,
	}
	cmd.SetHelpCommand(&cobra.Command{
		Hidden: true,
	})
	cmd.AddCommand(
		sync.NewRunCommand(),
	)
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	return cmd
}
