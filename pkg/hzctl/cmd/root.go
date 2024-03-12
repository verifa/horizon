package cmd

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

var rootCmd = &cobra.Command{
	Use:          "hzctl",
	Short:        "Horizon CLI for managing Horizon resources.",
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
			if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
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

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
