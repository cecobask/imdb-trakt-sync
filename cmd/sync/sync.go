package sync

import (
	"fmt"
	"os"

	"github.com/cecobask/imdb-trakt-sync/pkg/syncer"
	"github.com/spf13/cobra"
)

const CommandNameSync = "sync"

func NewRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", CommandNameSync),
		Short: "Sync IMDb data to Trakt",
		RunE: func(cmd *cobra.Command, args []string) error {
			return syncer.NewSyncer().Run()
		},
	}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	return cmd
}
