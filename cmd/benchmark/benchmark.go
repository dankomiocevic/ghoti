package benchmark

import (
	"fmt"
	"math/rand/v2"
	"net"
	"time"

	"github.com/dankomiocevic/ghoti/internal/config"

	"github.com/spf13/cobra"
)

func NewBenchmarkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "benchmark",
		Short: "Benchmark the Ghoti server",
		Long:  "Run an set of benchrmarks agains a Ghoti server.",
		Run:   run,
		Args:  cobra.NoArgs,
	}

	defaultConfig := config.DefaultConfig()
	flags := cmd.Flags()
	flags.String("addr", defaultConfig.TcpAddr, "the host:port address to serve the server on")

	return cmd
}

func run(cmd *cobra.Command, _ []string) {
	addr, _ := cmd.Flags().GetString("addr")

	fmt.Println("Starting connections..")
	var conns []net.Conn
	for i := 0; i < 1000; i++ {
		c, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			fmt.Println("Error to connect", i, err)
			continue
		}
		conns = append(conns, c)
		time.Sleep(time.Millisecond)
	}

	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	fmt.Printf("Enabled %d connections\n", len(conns))

	start := time.Now()
	for j := 0; j < 10000; j++ {
		if j%100 == 0 {
			duration := time.Since(start)
			tps := float64(j*1000) / duration.Seconds()
			fmt.Printf("Executed %d calls, elapsed %f seconds, %f tps\n", j*1000, duration.Seconds(), tps)
		}

		for i := 0; i < len(conns); i++ {
			conn := conns[i]
			if rand.IntN(100) < 5 {
				if rand.IntN(10) < 5 {
					conn.Write([]byte("w000test\n"))
				} else {
					conn.Write([]byte("w001test\n"))
				}
			} else {
				if rand.IntN(10) < 5 {
					conn.Write([]byte("r000test\n"))
				} else {
					conn.Write([]byte("r001test\n"))
				}
			}
		}
	}
}
