package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var contextGetCmd = &cobra.Command{
	Use:   "get",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.CurrentContext == "" {
			return fmt.Errorf("no current context")
		}
		hCtx, ok := config.Current()
		if !ok {
			return fmt.Errorf(
				"current context not found: %q",
				config.CurrentContext,
			)
		}
		fmt.Println(hCtx.Name, hCtx.URL)
		return nil
	},
}

func init() {
	contextCmd.AddCommand(contextGetCmd)

	contextGetCmd.Flags().
		StringVar(&contextSetCurrent, "current", "", "name of the current context")
}
