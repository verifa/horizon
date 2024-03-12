package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hzctl"
	"sigs.k8s.io/yaml"
)

type applyCmdOptions struct {
	filename string
}

var applyOpts applyCmdOptions

var applyCmd = &cobra.Command{
	Use:           "apply",
	Short:         "Server-side apply Horizon objects.",
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

		if applyOpts.filename == "" {
			return fmt.Errorf("filename is required")
		}

		yData, err := os.ReadFile(applyOpts.filename)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}

		jData, err := yaml.YAMLToJSONStrict(yData)
		if err != nil {
			return fmt.Errorf("convert yaml to json: %w", err)
		}

		client := hzctl.Client{
			Server:  hCtx.URL,
			Session: *hCtx.Session,
			Manager: "hzctl",
		}

		ctx := context.Background()
		if err := client.Apply(ctx, hzctl.WithApplyData(jData)); err != nil {
			return fmt.Errorf("apply: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)

	flags := applyCmd.Flags()
	flags.StringVarP(
		&applyOpts.filename,
		"filename",
		"f",
		"",
		"Filename to apply",
	)
}
