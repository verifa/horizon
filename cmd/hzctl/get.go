package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/hzctl"
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Args:          cobra.MinimumNArgs(1),
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
		var (
			objectType *string
			objectName *string
		)
		switch len(args) {
		case 0:
			return fmt.Errorf("at least one argument is required")
		case 1:
			objectType = &args[0]
		case 2:
			objectType = &args[0]
			objectName = &args[1]
		}
		key := hz.ObjectKey{}
		if objectType != nil {
			parts := strings.Split(*objectType, ".")
			switch len(parts) {
			case 1:
				key.Kind = parts[0]
			case 2:
				key.Kind = parts[0]
				key.Name = parts[1]
			default:
				return fmt.Errorf("invalid object type: %q", *objectType)
			}
		}
		if objectName != nil {
			key.Name = *objectName
		}

		client := hz.HTTPClient{
			Server:  "http://localhost:9999",
			Session: *hCtx.Session,
		}
		ctx := context.Background()
		resp := hz.GenericObjectList{}
		if err := client.List(
			ctx,
			hz.WithHTTPListKey(key),
			hz.WithHTTPListResponseGenericObject(&resp),
		); err != nil {
			return fmt.Errorf("list: %w", err)
		}
		if len(resp.Items) == 0 {
			fmt.Println("No objects found")
			return nil
		}

		if key.Name == "" {
			printObjects(resp.Items)
		} else {
			_ = printObject(resp.Items[0])
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
