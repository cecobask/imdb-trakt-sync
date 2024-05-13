package sync

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cecobask/imdb-trakt-sync/cmd"
	"github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/syncer"
)

func NewCommand() *cobra.Command {
	var conf *config.Config
	command := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", cmd.CommandNameSync),
		Short: "Sync IMDb data to Trakt",
		PreRunE: func(c *cobra.Command, args []string) (err error) {
			confPath, err := c.Flags().GetString(cmd.FlagNameConfigFile)
			if err != nil {
				return err
			}
			if conf, err = config.New(confPath, true); err != nil {
				return fmt.Errorf("error loading config: %w", err)
			}
			return conf.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			s, err := syncer.NewSyncer(conf)
			if err != nil {
				return fmt.Errorf("error creating syncer: %w", err)
			}
			return s.Sync()
		},
	}
	command.Flags().String(cmd.FlagNameConfigFile, cmd.ConfigFileDefault, "path to the config file")
	return command
}
