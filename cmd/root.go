// Package cmd contains all the commands included in the binary file.
package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewRootCommand enables all children commands to read flags from CLI flags, environment variables prefixed with GHOTI, or config.yaml (in that order).
//
// The key replacer maps both dashes and dots to underscores so that nested
// configuration keys can be set from the environment: cluster.pass is read
// from GHOTI_CLUSTER_PASS. This is what allows a credential to be injected at
// runtime instead of being written into a configuration file.
func NewRootCommand() *cobra.Command {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("GHOTI")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	configPaths := []string{"/etc/ghoti", "$HOME/.ghoti", "."}
	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	var configFile string

	cmd := &cobra.Command{
		Use:   "ghoti",
		Short: "A simple server to do simple things, but fast!",
		Long: `Ghoti is a server that performs simple tasks in a reliable and fast way.

Distributed systems are complicated, sometimes is good to have a centralized way to perform some tasks to simplify the overall architecture.`,
		// The flag value is only known after parsing, so the config file has
		// to be set here and not when the command is created.
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			if configFile != "" {
				viper.SetConfigFile(configFile)
			}
		},
	}

	cmd.PersistentFlags().StringVar(&configFile, "config", "", "path to the configuration file, replaces the default search paths (/etc/ghoti, $HOME/.ghoti and the current directory)")

	return cmd
}
