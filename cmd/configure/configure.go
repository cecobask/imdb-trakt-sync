package configure

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/cecobask/imdb-trakt-sync/cmd"
	config2 "github.com/cecobask/imdb-trakt-sync/internal/config"
)

func NewCommand() *cobra.Command {
	var conf *config2.Config
	var confPath string
	command := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", cmd.CommandNameConfigure),
		Short: "Configure provider credentials and sync options",
		PreRunE: func(c *cobra.Command, args []string) (err error) {
			confPath, err = c.Flags().GetString(cmd.FlagNameConfigFile)
			if err != nil {
				return err
			}
			if conf, err = config2.New(confPath, false); err != nil {
				return fmt.Errorf("error loading config: %w", err)
			}
			return conf.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			teaModel, err := config2.NewTeaProgram(conf.Flatten(), tea.WithOutput(c.OutOrStdout())).Run()
			if err != nil {
				return fmt.Errorf("error initializing text-based user interface for the %s command: %w", cmd.CommandNameConfigure, err)
			}
			model, ok := teaModel.(*config2.Model)
			if !ok {
				return fmt.Errorf("error type asserting tea.Model to *config.Model")
			}
			if err = model.Err(); err != nil {
				if errors.Is(err, config2.ErrUserAborted) {
					return nil
				}
				return fmt.Errorf("error occurred in the config model: %w", err)
			}
			if conf, err = config2.NewFromMap(model.Config()); err != nil {
				return fmt.Errorf("error loading config from map: %w", err)
			}
			if err = conf.Validate(); err != nil {
				return fmt.Errorf("error validating config: %w", err)
			}
			return conf.WriteFile(confPath)
		},
	}
	command.Flags().String(cmd.FlagNameConfigFile, cmd.ConfigFileDefault, "path to the config file")
	return command
}
