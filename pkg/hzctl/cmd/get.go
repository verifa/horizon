package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/hzctl"
)

var getCmd = &cobra.Command{
	Use:           "get",
	Short:         "Get Horizon objects.",
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

		client := hzctl.Client{
			Server:  hCtx.URL,
			Session: *hCtx.Session,
		}
		ctx := context.Background()
		resp := hz.GenericObjectList{}
		if err := client.List(
			ctx,
			hzctl.WithListKey(key),
			hzctl.WithListResponseGenericObject(&resp),
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
