package root

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/cecobask/imdb-trakt-sync/cmd"
	"github.com/cecobask/imdb-trakt-sync/cmd/configure"
	"github.com/cecobask/imdb-trakt-sync/cmd/sync"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:     cmd.CommandNameRoot,
		Aliases: []string{cmd.CommandAliasRoot},
		Short:   "imdb-trakt-sync command line interface",
		PersistentPreRun: func(c *cobra.Command, args []string) {
			c.SetOut(os.Stdout)
			c.SetErr(os.Stderr)
		},
		RunE: func(c *cobra.Command, args []string) error {
			return c.Help()
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
		configure.NewCommand(),
		sync.NewCommand(),
	)
	command.SetOut(os.Stdout)
	command.SetErr(os.Stderr)
	return command
}
