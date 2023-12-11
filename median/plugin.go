package median

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2plus/types"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/loop"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

const contractName = "median"

type Plugin struct {
	loop.Plugin
	stop services.StopChan
}

func NewPlugin(lggr logger.Logger) *Plugin {
	return &Plugin{Plugin: loop.Plugin{Logger: lggr}, stop: make(services.StopChan)}
}

func (p *Plugin) NewMedianFactory(ctx context.Context, provider types.MedianProvider, dataSource, juelsPerFeeCoin median.DataSource, errorLog loop.ErrorLog) (loop.ReportingPluginFactory, error) {
	var ctxVals loop.ContextValues
	ctxVals.SetValues(ctx)
	lggr := logger.With(p.Logger, ctxVals.Args()...)

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
	}

	if cr := provider.ChainReader(); cr != nil {
		factory.ContractTransmitter = &chainReaderContract{chainReader: cr}
	} else {
		factory.ContractTransmitter = provider.MedianContract()
	}

	if codec := provider.Codec(); codec != nil {
		var err error
		if factory.ReportCodec, err = newReportCodec(codec); err != nil {
			return nil, err
		}
	} else {
		lggr.Warn("No codec provided, defaulting back to median specific ReportCodec")
		factory.ReportCodec = provider.ReportCodec()
	}

	s := &reportingPluginFactoryService{lggr: logger.Named(lggr, "ReportingPluginFactory"), ReportingPluginFactory: factory}

	p.SubService(s)

	return s, nil
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

	err = c.chainReader.GetLatestValue(ctx, contractName, "LatestTransmissionDetails", nil, &resp)
	if err != nil && !errors.Is(err, types.ErrNotFound) {
		return
	}

	// Depending on if there is a LatestAnswer or not, and the implementation of the ChainReader,
	// it's possible that this will be unset. The desired behaviour in that case is to have a zero value.
	if resp.LatestAnswer == nil {
		resp.LatestAnswer = new(big.Int)
	}

	return resp.ConfigDigest, resp.Epoch, resp.Round, resp.LatestAnswer, resp.LatestTimestamp, nil
}

func (c *chainReaderContract) LatestRoundRequested(ctx context.Context, lookback time.Duration) (configDigest ocrtypes.ConfigDigest, epoch uint32, round uint8, err error) {
	var resp latestRoundRequested

	err = c.chainReader.GetLatestValue(ctx, contractName, "LatestRoundRequested", nil, &resp)
	if err != nil && !errors.Is(err, types.ErrNotFound) {
		return
	}

	return resp.ConfigDigest, resp.Epoch, resp.Round, nil
}
