package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/hzctl"
	"sigs.k8s.io/yaml"
)

type deleteCmdOptions struct {
	filename string
	key      string
}

var deleteOpts deleteCmdOptions

var deleteCmd = &cobra.Command{
	Use:           "delete",
	Short:         "Mark Horizon objects for deletion.",
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

		clientDeleteOpts := []hzctl.DeleteOption{}
		if deleteOpts.filename != "" {
			yData, err := os.ReadFile(deleteOpts.filename)
			if err != nil {
				return fmt.Errorf("open file: %w", err)
			}

			jData, err := yaml.YAMLToJSONStrict(yData)
			if err != nil {
				return fmt.Errorf("convert yaml to json: %w", err)
			}
			clientDeleteOpts = append(
				clientDeleteOpts,
				hzctl.WithDeleteData(jData),
			)
		}
		if deleteOpts.key != "" {
			objKey, err := hz.ObjectKeyFromString(deleteOpts.key)
			if err != nil {
				return fmt.Errorf("parse key: %w", err)
			}
			clientDeleteOpts = append(
				clientDeleteOpts,
				hzctl.WithDeleteKey(objKey),
			)
		}

		client := hzctl.Client{
			Server:  hCtx.URL,
			Session: *hCtx.Session,
			Manager: "hzctl",
		}

		ctx := context.Background()
		if err := client.Delete(ctx, clientDeleteOpts...); err != nil {
			return fmt.Errorf("delete: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	flags := deleteCmd.Flags()
	flags.StringVarP(
		&deleteOpts.filename,
		"filename",
		"f",
		"",
		"Filename to delete",
	)
	flags.StringVarP(
		&deleteOpts.key,
		"key",
		"k",
		"",
		"Key to delete",
	)
}
