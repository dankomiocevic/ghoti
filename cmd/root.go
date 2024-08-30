// Package cmd contains all the commands included in the binary file.
package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewRootCommand enables all children commands to read flags from CLI flags, environment variables prefixed with GHOTI, or config.yaml (in that order).
func NewRootCommand() *cobra.Command {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("GHOTI")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	configPaths := []string{"/etc/ghoti", "$HOME/.ghoti", "."}
	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}

	return &cobra.Command{
		Use:   "ghoti",
		Short: "A simple server to do simple things, but fast!",
		Long: `Ghoti is a server that performs simple tasks in a reliable and fast way.

Distributed systems are complicated, sometimes is good to have a centralized way to perform some tasks to simplify the overall architecture.`,
	}
}
