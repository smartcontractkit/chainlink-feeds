package median

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2plus/types"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/loop"
	"github.com/smartcontractkit/chainlink-common/pkg/loop/reportingplugins"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Plugin struct {
	loop.Plugin
	stop services.StopChan
	reportingplugins.MedianProviderServer
}

func NewPlugin(lggr logger.Logger) *Plugin {
	return &Plugin{
		Plugin:               loop.Plugin{Logger: lggr},
		MedianProviderServer: reportingplugins.MedianProviderServer{},
		stop:                 make(services.StopChan),
	}
}

func (p *Plugin) NewMedianFactory(ctx context.Context, provider types.MedianProvider, dataSource, juelsPerFeeCoin median.DataSource, errorLog loop.ErrorLog) (loop.ReportingPluginFactory, error) {
	var ctxVals loop.ContextValues
	ctxVals.SetValues(ctx)
	lggr := logger.With(p.Logger, ctxVals.Args()...)
	factory, err := p.newMedianFactory(lggr, provider, dataSource, juelsPerFeeCoin, errorLog)
	if err != nil {
		return nil, err
	}

	s := &reportingPluginFactoryService{lggr: logger.Named(lggr, "ReportingPluginFactory"), ReportingPluginFactory: factory}

	p.SubService(s)

	return s, nil
}

func (p *Plugin) newMedianFactory(lggr logger.Logger, provider types.MedianProvider, dataSource, juelsPerFeeCoin median.DataSource, errorLog loop.ErrorLog) (median.NumericalMedianFactory, error) {
	factory := median.NumericalMedianFactory{
		DataSource:                dataSource,
		JuelsPerFeeCoinDataSource: juelsPerFeeCoin,
		Logger: logger.NewOCRWrapper(lggr, true, func(msg string) {
			ctx, cancelFn := p.stop.NewCtx()
			defer cancelFn()
			if err := errorLog.SaveError(ctx, msg); err != nil {
				lggr.Errorw("Unable to save error", "err", msg)
			}
		}),
		OnchainConfigCodec: provider.OnchainConfigCodec(),
		ReportCodec:        provider.ReportCodec(),
	}
	if cr := provider.ChainReader(); cr != nil {
		factory.ContractTransmitter = &chainReaderContract{cr, types.BoundContract{Name: "median"}}
	} else {
		factory.ContractTransmitter = provider.MedianContract()
	}
	return factory, nil
}

type pipelineSpec struct {
	Name string `json:"name"`
	Spec string `json:"spec"`
}

type jsonConfig struct {
	Pipelines []pipelineSpec `json:"pipelines"`
}

func (j jsonConfig) defaultPipeline() (string, error) {
	return j.getPipeline("__DEFAULT_PIPELINE__")
}

func (j jsonConfig) getPipeline(key string) (string, error) {
	for _, v := range j.Pipelines {
		if v.Name == key {
			return v.Spec, nil
		}
	}
	return "", fmt.Errorf("no pipeline found for %s", key)
}

func (p *Plugin) NewReportingPluginFactory(
	ctx context.Context,
	config types.ReportingPluginServiceConfig,
	provider types.MedianProvider,
	pipelineRunner types.PipelineRunnerService,
	telemetry types.TelemetryClient,
	errorLog types.ErrorLog,
) (types.ReportingPluginFactory, error) {
	f, err := p.newFactory(ctx, config, provider, pipelineRunner, telemetry, errorLog)
	if err != nil {
		return nil, err
	}
	s := &reportingPluginFactoryService{lggr: p.Logger, ReportingPluginFactory: f}
	p.SubService(s)
	return s, nil
}

func (p *Plugin) newFactory(ctx context.Context, config types.ReportingPluginServiceConfig, provider types.MedianProvider, pipelineRunner types.PipelineRunnerService, telemetry types.TelemetryClient, errorLog types.ErrorLog) (median.NumericalMedianFactory, error) {
	jc := &jsonConfig{}
	err := json.Unmarshal([]byte(config.PluginConfig), jc)
	if err != nil {
		return median.NumericalMedianFactory{}, err
	}

	dp, err := jc.defaultPipeline()
	if err != nil {
		return median.NumericalMedianFactory{}, err
	}
	ds := &DataSource{
		pipelineRunner: pipelineRunner,
		spec:           dp,
		lggr:           p.Logger,
	}

	jfp, err := jc.getPipeline("juelsPerFeeCoinPipeline")
	if err != nil {
		return median.NumericalMedianFactory{}, err
	}
	jds := &DataSource{
		pipelineRunner: pipelineRunner,
		spec:           jfp,
		lggr:           p.Logger,
	}

	return p.newMedianFactory(p.Logger, provider, ds, jds, errorLog)
}

type reportingPluginFactoryService struct {
	services.StateMachine
	lggr logger.Logger
	ocrtypes.ReportingPluginFactory
}

func (r *reportingPluginFactoryService) Name() string { return r.lggr.Name() }

func (r *reportingPluginFactoryService) Start(ctx context.Context) error {
	return r.StartOnce("ReportingPluginFactory", func() error { return nil })
}

func (r *reportingPluginFactoryService) Close() error {
	return r.StopOnce("ReportingPluginFactory", func() error { return nil })
}

func (r *reportingPluginFactoryService) HealthReport() map[string]error {
	return map[string]error{r.Name(): r.Healthy()}
}

// chainReaderContract adapts a [types.ChainReader] to [median.MedianContract].
type chainReaderContract struct {
	chainReader types.ChainReader
	contract    types.BoundContract
}

type latestTransmissionDetailsResponse struct {
	ConfigDigest    ocrtypes.ConfigDigest
	Epoch           uint32
	Round           uint8
	LatestAnswer    *big.Int
	LatestTimestamp time.Time
}

type latestRoundRequested struct {
	ConfigDigest ocrtypes.ConfigDigest
	Epoch        uint32
	Round        uint8
}

func (c *chainReaderContract) LatestTransmissionDetails(ctx context.Context) (configDigest ocrtypes.ConfigDigest, epoch uint32, round uint8, latestAnswer *big.Int, latestTimestamp time.Time, err error) {
	var resp latestTransmissionDetailsResponse

	err = c.chainReader.GetLatestValue(ctx, c.contract, "LatestTransmissionDetails", nil, &resp)
	if err != nil {
		return
	}

	return resp.ConfigDigest, resp.Epoch, resp.Round, resp.LatestAnswer, resp.LatestTimestamp, nil
}

func (c *chainReaderContract) LatestRoundRequested(ctx context.Context, lookback time.Duration) (configDigest ocrtypes.ConfigDigest, epoch uint32, round uint8, err error) {
	var resp latestRoundRequested

	err = c.chainReader.GetLatestValue(ctx, c.contract, "LatestRoundReported", map[string]any{"lookback": lookback}, &resp)
	if err != nil {
		return
	}

	return resp.ConfigDigest, resp.Epoch, resp.Round, nil
}
