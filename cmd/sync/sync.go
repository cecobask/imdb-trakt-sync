package sync

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cecobask/imdb-trakt-sync/cmd"
	"github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/syncer"
)

func NewCommand(ctx context.Context) *cobra.Command {
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
			if err = conf.Validate(); err != nil {
				return fmt.Errorf("error validating config: %w", err)
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			timeoutCtx, cancel := context.WithTimeout(ctx, *conf.Sync.Timeout)
			defer cancel()
			s, err := syncer.NewSyncer(timeoutCtx, conf)
			if err != nil {
				return fmt.Errorf("error creating syncer: %w", err)
			}
			if err = s.Sync(); err != nil {
				return fmt.Errorf("error performing sync: %w", err)
			}
			return nil
		},
	}
	command.Flags().String(cmd.FlagNameConfigFile, cmd.ConfigFileDefault, "path to the config file")
	return command
}
