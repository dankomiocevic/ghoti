// Package run contains the command to run an instance of a Ghoti server.
package run

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dankomiocevic/ghoti/internal/cluster"
	"github.com/dankomiocevic/ghoti/internal/config"
	"github.com/dankomiocevic/ghoti/internal/server"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Ghoti server",
		Long:  "Run an instance of Ghoti.",
		Run:   run,
		Args:  cobra.NoArgs,
	}

	defaultConfig := config.DefaultConfig()
	flags := cmd.Flags()
	flags.String("addr", defaultConfig.TcpAddr, "the host:port address to serve the server on")
	viper.BindPFlag("addr", cmd.Flags().Lookup("addr"))

	return cmd
}

func run(_ *cobra.Command, _ []string) {
	config, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	if err := config.Verify(); err != nil {
		panic(err)
	}

	if len(config.Cluster.Node) > 0 {
		c, err := cluster.NewCluster(config.Cluster)
		if err != nil {
			panic(err)
		}
		c.Start()
	}

	s := server.NewServer(config)
	defer s.Stop()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	fmt.Println("Shutting down server..")
}
