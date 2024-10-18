package benchmark

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/dankomiocevic/ghoti/cmd"
)

func TestNoServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test on Short mode")
	}

	rootCmd := cmd.NewRootCommand()
	cmd := NewBenchmarkCommand()
	rootCmd.AddCommand(cmd)

	rootCmd.SetArgs([]string{"benchmark", "--addr", "localhost:9993"})

	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)

	rootCmd.Execute()
	out, err := ioutil.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Enabled 0 connections") {
		t.Fatalf("Command output does not contain expected string: %s", string(out))
	}
}
