package run

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
)

type DummyExit struct {
	mock.Mock
}

func (d *DummyExit) Exit(code int) {
	d.Called(code)
	return
}

func TestNoConfig(t *testing.T) {
	viper.Reset()
	viper.AddConfigPath("whattayoutalkingbout")
	viper.SetConfigName("wrong_name")
	viper.SetConfigType("yaml")

	e := new(DummyExit)
	e.On("Exit", 1).Return()
	runWithExit(e)

	e.AssertExpectations(t)
}

func TestClusterFail(t *testing.T) {
	viper.Reset()
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("GHOTI")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	configPaths := []string{"/etc/ghoti", "$HOME/.ghoti", ".", "../.."}
	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	viper.Set("cluster.node", "node1")
	viper.Set("cluster.user", "a")
	viper.Set("cluster.pass", "b")
	viper.Set("cluster.manager.type", "pepe")
	viper.Set("cluster.manager.join", "pepe")
	viper.Set("cluster.manager.addr", "pepe")

	e := new(DummyExit)
	e.On("Exit", 3).Return()
	runWithExit(e)

	e.AssertExpectations(t)
}
