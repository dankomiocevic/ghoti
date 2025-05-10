// Package main contains the root of all commands.
package main

import (
	"os"

	"github.com/dankomiocevic/ghoti/cmd"
	"github.com/dankomiocevic/ghoti/cmd/benchmark"
	"github.com/dankomiocevic/ghoti/cmd/run"
)

func main() {
	rootCmd := cmd.NewRootCommand()

	runCmd := run.NewRunCommand()
	rootCmd.AddCommand(runCmd)

	benchmarkCmd := benchmark.NewBenchmarkCommand()
	rootCmd.AddCommand(benchmarkCmd)

	versionCmd := cmd.NewVersionCommand()
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
