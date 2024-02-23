package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hz"
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
		hCtx, ok := config.Current()
		if !ok {
			return fmt.Errorf(
				"current context not found: %q",
				config.CurrentContext,
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
				key.Group = parts[0]
				key.Kind = parts[1]
			default:
				return fmt.Errorf("invalid object type: %q", *objectType)
			}
		}
		if objectName != nil {
			key.Name = *objectName
		}

		if hCtx.Credentials == nil {
			return fmt.Errorf(
				"no credentials for context: %q. Please login",
				hCtx.Name,
			)
		}
		if hCtx.Session == nil {
			return fmt.Errorf(
				"no session for context: %q. Please login",
				hCtx.Name,
			)
		}
		creds, err := base64.StdEncoding.DecodeString(*hCtx.Credentials)
		if err != nil {
			return fmt.Errorf("decode credentials: %w", err)
		}
		userJWT, err := nkeys.ParseDecoratedJWT(creds)
		if err != nil {
			return fmt.Errorf("parse jwt: %w", err)
		}
		keyPair, err := nkeys.ParseDecoratedUserNKey(creds)
		if err != nil {
			return fmt.Errorf("parse nkey: %w", err)
		}
		seed, err := keyPair.Seed()
		if err != nil {
			return fmt.Errorf("get seed: %w", err)
		}
		conn, err := nats.Connect(
			hCtx.URL,
			nats.UserJWTAndSeed(userJWT, string(seed)),
		)
		if err != nil {
			return fmt.Errorf("connect to nats: %w", err)
		}
		client := hz.NewClient(
			conn,
			hz.WithClientManager("hzctl"),
			hz.WithClientSession(*hCtx.Session),
		)
		ctx := context.Background()
		data, err := client.List(ctx, hz.WithListKeyFromObject(key))
		if err != nil {
			return fmt.Errorf("list: %w", err)
		}
		fmt.Println(string(data))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
