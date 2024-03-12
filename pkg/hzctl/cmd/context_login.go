package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hzctl/login"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to Horizon to get a session token.",
	RunE: func(cmd *cobra.Command, args []string) error {
		hCtx, err := config.Context()
		if err != nil {
			return fmt.Errorf(
				"obtaining current context: %w",
				err,
			)
		}
		ctx := context.Background()
		resp, err := login.Login(ctx, login.LoginRequest{
			URL: hCtx.URL,
		})
		if err != nil {
			return err
		}
		hCtx.Session = &resp.Session
		config.Add(hCtx)
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
	contextCmd.AddCommand(loginCmd)
}
