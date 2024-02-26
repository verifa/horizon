package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hzctl"
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
		hCtx, err := config.Context(
			hzctl.WithContextCurrent(true),
			hzctl.WithContextValidate(hzctl.WithValidateSession(true)),
		)
		if err != nil {
			return fmt.Errorf(
				"obtaining current context: %w",
				err,
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
