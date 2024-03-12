package cmd

import (
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage Horizon contexts",
	Long:  `Context are used to manage different Horizon servers / environments.`,
}

func init() {
	rootCmd.AddCommand(contextCmd)
}
