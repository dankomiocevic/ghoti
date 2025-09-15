// Package run contains the command to run an instance of a Ghoti server.
package run

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dankomiocevic/ghoti/internal/cluster"
	"github.com/dankomiocevic/ghoti/internal/config"
	"github.com/dankomiocevic/ghoti/internal/server"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ExitControl interface {
	Exit(int)
}

type ExitControlCmd struct {
}

func (e *ExitControlCmd) Exit(code int) {
	os.Exit(code)
}

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
	e := &ExitControlCmd{}

	runWithExit(e)
}

func runWithExit(e ExitControl) {
	config, err := config.LoadConfig()
	if err != nil {
		slog.Error("Error loading config",
			slog.Any("error", err),
		)
		e.Exit(1)
		return
	}

	if err := config.Verify(); err != nil {
		slog.Error("Error verifying config",
			slog.Any("error", err),
		)
		e.Exit(2)
		return
	}

	createLogger(config)

	var clus cluster.Cluster
	if len(config.Cluster.Node) > 0 {
		clus, err = cluster.NewCluster(config.Cluster)
		if err != nil {
			slog.Error("Error starting cluster",
				slog.Any("error", err),
			)
			e.Exit(3)
			return
		}
		clus.Start()
	} else {
		clus = cluster.NewEmptyCluster()
	}

	s := server.NewServer(config, clus)
	defer s.Stop()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	slog.Info("Shutting down server")
}

func createLogger(conf *config.Config) {
	opts := &slog.HandlerOptions{
		Level: conf.Logging.Level,
	}

	var logger *slog.Logger
	switch conf.Logging.Format {
	case "json":
		logger = slog.New(slog.NewJSONHandler(os.Stdout, opts))
	default:
		logger = slog.New(slog.NewTextHandler(os.Stdout, opts))
	}

	slog.SetDefault(logger)
}
