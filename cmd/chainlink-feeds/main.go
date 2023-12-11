package main

import (
	"github.com/hashicorp/go-plugin"

	"github.com/smartcontractkit/chainlink-common/pkg/loop"
	"github.com/smartcontractkit/chainlink-common/pkg/loop/reportingplugins"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-feeds/median"
)

const (
	loggerName = "PluginMedian"
)

func main() {
	s := loop.MustNewStartedServer(loggerName)
	defer s.Stop()

	p := median.NewPlugin(s.Logger)
	defer s.Logger.ErrorIfFn(p.Close, "Failed to close")

	s.MustRegister(p)

	stop := make(chan struct{})
	defer close(stop)

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: loop.PluginMedianHandshakeConfig(),
		Plugins: map[string]plugin.Plugin{
			loop.PluginMedianName: &loop.GRPCPluginMedian{
				PluginServer: p,
				BrokerConfig: loop.BrokerConfig{
					StopCh:   stop,
					Logger:   s.Logger,
					GRPCOpts: s.GRPCOpts,
				},
			},
			reportingplugins.PluginServiceName: &reportingplugins.GRPCService[types.MedianProvider]{
				PluginServer: p,
				BrokerConfig: loop.BrokerConfig{
					Logger:   s.Logger,
					StopCh:   stop,
					GRPCOpts: s.GRPCOpts,
				},
			},
		},
		GRPCServer: s.GRPCOpts.NewServer,
	})
}
