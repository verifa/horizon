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
		// If using nats instead of http, below is how to create a nats client.
		// Leaving it here as not sure which approach we are going with just yet
		// (or maybe both!).
		// creds, err := base64.StdEncoding.DecodeString(*hCtx.Credentials)
		// if err != nil {
		// 	return fmt.Errorf("decode credentials: %w", err)
		// }
		// userJWT, err := nkeys.ParseDecoratedJWT(creds)
		// if err != nil {
		// 	return fmt.Errorf("parse jwt: %w", err)
		// }
		// keyPair, err := nkeys.ParseDecoratedUserNKey(creds)
		// if err != nil {
		// 	return fmt.Errorf("parse nkey: %w", err)
		// }
		// seed, err := keyPair.Seed()
		// if err != nil {
		// 	return fmt.Errorf("get seed: %w", err)
		// }
		// conn, err := nats.Connect(
		// 	hCtx.URL,
		// 	nats.UserJWTAndSeed(userJWT, string(seed)),
		// )
		// if err != nil {
		// 	return fmt.Errorf("connect to nats: %w", err)
		// }
		// client := hz.NewClient(
		// 	conn,
		// 	hz.WithClientManager("hzctl"),
		// 	hz.WithClientSession(*hCtx.Session),
		// )
		// ctx := context.Background()
		// resp := hz.GenericObjectList{}
		// if err := client.List(ctx, hz.WithListKeyFromObject(key),
		// hz.WithListResponseGenericObjects(&resp)); err != nil {
		// 	return fmt.Errorf("list: %w", err)
		// }
		switch len(resp.Items) {
		case 0:
			fmt.Println("No objects found")
		case 1:
			if err := printObject(resp.Items[0]); err != nil {
				return fmt.Errorf("print object: %w", err)
			}
		default:
			printObjects(resp.Items)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
