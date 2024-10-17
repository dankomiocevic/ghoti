package cmd

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	rootCmd := NewRootCommand()
	verCmd := NewVersionCommand()
	rootCmd.AddCommand(verCmd)

	rootCmd.SetArgs([]string{"version"})

	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)

	rootCmd.Execute()
	out, err := ioutil.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Ghoti version") {
		t.Fatalf("Command output does not contain expected string: %s", out)
	}
}
