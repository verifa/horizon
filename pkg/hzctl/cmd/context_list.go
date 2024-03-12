package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var contextListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List all available contexts.",
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, hCtx := range config.Contexts {
			fmt.Println(hCtx.Name, hCtx.URL)
		}
		return nil
	},
}

func init() {
	contextCmd.AddCommand(contextListCmd)
}
