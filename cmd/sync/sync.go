package sync

import (
	"fmt"

	"github.com/cecobask/imdb-trakt-sync/pkg/config"
	"github.com/cecobask/imdb-trakt-sync/pkg/syncer"
	"github.com/spf13/cobra"
)

const CommandNameSync = "sync"

func NewSyncCommand() *cobra.Command {
	var conf *config.Config
	command := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", CommandNameSync),
		Short: "Sync IMDb data to Trakt",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			configPath, err := cmd.Flags().GetString("config-file")
			if err != nil {
				return err
			}
			if conf, err = config.New(configPath, true); err != nil {
				return fmt.Errorf("error loading config: %w", err)
			}
			return conf.Validate()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := syncer.NewSyncer(conf)
			if err != nil {
				return fmt.Errorf("error creating syncer: %w", err)
			}
			return s.Sync()
		},
	}
	command.Flags().String("config-file", "config.yaml", "path to the config file")
	return command
}
