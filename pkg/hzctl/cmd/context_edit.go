package cmd

import (
	"fmt"
	"os"
	"regexp"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hzctl"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

type contextEditOptions struct {
	name string
	url  string
}

var contextEditOpts contextEditOptions

var contextEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		hCtx, err := config.Context(
			hzctl.WithContextTryName(&contextEditOpts.name),
			hzctl.WithContextValidate(hzctl.WithValidateSession(true)),
		)
		if err != nil {
			return fmt.Errorf(
				"obtaining context: %w",
				err,
			)
		}
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Context name").
					Value(&hCtx.Name).
					Validate(func(str string) error {
						matched, err := regexp.MatchString(
							"^[a-zA-Z0-9_-]+$",
							str,
						)
						if err != nil {
							return fmt.Errorf("match string: %w", err)
						}
						if !matched {
							return fmt.Errorf("invalid context name")
						}
						return nil
					}),
				huh.NewInput().
					Title("Horizon Server URL").
					Placeholder("http://localhost:9999").
					Value(&hCtx.URL).
					Validate(func(str string) error {
						return nil
					}),
			),
		).WithTheme(huh.ThemeCatppuccin())
		if err := form.Run(); err != nil {
			return fmt.Errorf("form run: %w", err)
		}
		config.Add(hCtx)

		f, err := os.Create(configFile)
		if err != nil {
			return fmt.Errorf("create config file: %w", err)
		}
		defer f.Close()
		if err := yaml.NewEncoder(f).Encode(config); err != nil {
			return fmt.Errorf("encode config: %w", err)
		}
		return nil
	},
}

func init() {
	contextCmd.AddCommand(contextEditCmd)

	contextEditCmd.Flags().
		StringVar(&contextEditOpts.name, "name", "", "name of the context")
	contextEditCmd.Flags().
		StringVar(&contextEditOpts.url, "url", "", "url of the horizon server")
}
