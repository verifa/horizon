package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

var contextSetCurrent string

var contextSetCmd = &cobra.Command{
	Use:   "set",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		isModified := false
		if contextSetCurrent != "" {
			if config.CurrentContext != contextSetCurrent {
				config.CurrentContext = contextSetCurrent
				isModified = true
			}
		}

		if !isModified {
			return nil
		}

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
	contextCmd.AddCommand(contextSetCmd)

	contextSetCmd.Flags().
		StringVar(&contextSetCurrent, "current", "", "name of the current context")
}
