package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newTestRoot returns a root command with a child that does nothing, so the
// persistent pre-run hook is executed without starting a server.
func newTestRoot() *cobra.Command {
	viper.Reset()
	rootCmd := NewRootCommand()
	rootCmd.AddCommand(&cobra.Command{
		Use: "noop",
		Run: func(_ *cobra.Command, _ []string) {},
	})
	return rootCmd
}

func TestConfigFlagSetsConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.yml")
	if err := os.WriteFile(path, []byte("addr: localhost:1234\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rootCmd := newTestRoot()
	rootCmd.SetArgs([]string{"noop", "--config", path})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if got := viper.ConfigFileUsed(); got != path {
		t.Fatalf("expected config file %s, got %s", path, got)
	}
	if got := viper.GetString("addr"); got != "localhost:1234" {
		t.Fatalf("expected addr localhost:1234, got %s", got)
	}
}

func TestConfigFlagMissingFileDoesNotFallBack(t *testing.T) {
	// The current directory of this package has no config.yaml, so add a
	// search path that has one to prove the flag replaces the search paths.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("addr: localhost:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rootCmd := newTestRoot()
	viper.AddConfigPath(dir)
	rootCmd.SetArgs([]string{"noop", "--config", filepath.Join(dir, "missing.yaml")})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if err := viper.ReadInConfig(); err == nil {
		t.Fatalf("expected an error for a missing config file, read %s", viper.ConfigFileUsed())
	}
}

func TestNoConfigFlagUsesSearchPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("addr: localhost:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rootCmd := newTestRoot()
	viper.AddConfigPath(dir)
	rootCmd.SetArgs([]string{"noop"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if got := viper.ConfigFileUsed(); got != path {
		t.Fatalf("expected config file %s, got %s", path, got)
	}
}
