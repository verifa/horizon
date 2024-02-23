package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/verifa/horizon/pkg/hzctl"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

var (
	configFile string
	config     hzctl.Config
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "horizon",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting user home dir: %w", err)
		}
		configFile = filepath.Join(home, ".config", "horizon", "config.yaml")
		f, err := os.Open(configFile)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("open config file: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
				return fmt.Errorf("mkdir config file: %w", err)
			}
			return nil
		}
		defer f.Close()
		if err := yaml.NewDecoder(f).Decode(&config); err != nil {
			return fmt.Errorf("decode config: %w", err)
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags
// appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file
	// (default is $HOME/.horizon.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func main() {
	Execute()
}
