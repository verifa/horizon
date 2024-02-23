package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hzctl"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

var (
	contextName string
	contextURL  string
)

var contextAddCmd = &cobra.Command{
	Use:   "add",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Context name").
					Value(&contextName).
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
					Title("NATS Server URL").
					Placeholder("nats://localhost:4222").
					Value(&contextURL).
					Validate(func(str string) error {
						return nil
					}),
			),
		).WithTheme(huh.ThemeCatppuccin())
		if err := form.Run(); err != nil {
			return fmt.Errorf("form run: %w", err)
		}
		if contextName == "" {
			return fmt.Errorf("context name is required")
		}
		if contextURL == "" {
			return fmt.Errorf("context url is required")
		}
		hzCtx := hzctl.Context{
			Name: contextName,
			URL:  contextURL,
		}
		config.Add(hzCtx)

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
	contextCmd.AddCommand(contextAddCmd)

	contextAddCmd.Flags().
		StringVar(&contextName, "name", "", "name of the context")
	contextAddCmd.Flags().
		StringVar(&contextURL, "url", "", "url of the nats server")
}
