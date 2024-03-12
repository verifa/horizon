package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

type contextSetCmdOptions struct {
	current string
}

var contextSetOptions contextSetCmdOptions

var contextSetCmd = &cobra.Command{
	Use:           "set",
	Short:         "Set the current context.",
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		isModified := false
		if contextSetOptions.current != "" {
			if config.CurrentContext != contextSetOptions.current {
				config.CurrentContext = contextSetOptions.current
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
		StringVar(&contextSetOptions.current, "current", "", "name of the current context")
}
