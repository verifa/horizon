package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hzctl"
)

// type contextGetCmdOptions struct{}

// var contextGetOptions contextGetCmdOptions

var contextGetCmd = &cobra.Command{
	Use:           "get",
	Short:         "Get information about the current context.",
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
}
