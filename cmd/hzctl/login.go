package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hzctl/login"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

// loginCmd represents the login command
var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
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
			URL: "http://localhost:9999",
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
	authCmd.AddCommand(loginCmd)
}
