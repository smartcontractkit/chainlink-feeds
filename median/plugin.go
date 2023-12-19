package median

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"time"

	"github.com/google/go-cmp/cmp"
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
		factory.ContractTransmitter = &chainReaderContract{chainReader: cr, old: provider.MedianContract(), lggr: lggr}
	} else {
		factory.ContractTransmitter = provider.MedianContract()
	}

	if codec := provider.Codec(); codec != nil {
		lggr.Infof("!!!!!!!!\nnew codec in use\n!!!!!!!!")
		var err error
		if factory.ReportCodec, err = newReportCodec(codec); err != nil {
			return nil, err
		}
	} else {
		lggr.Warn("!!!!!!!!\nNo codec provided, defaulting back to median specific ReportCodec\n!!!!!!!!")
		factory.ReportCodec = provider.ReportCodec()
	}

	factory.ReportCodec = &wrapper{rc: factory.ReportCodec, old: provider.ReportCodec(), lggr: lggr}

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
	old         median.MedianContract
	lggr        logger.Logger
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

func (m *chainReaderContract) LatestTransmissionDetails(ctx context.Context) (configDigest ocrtypes.ConfigDigest, epoch uint32, round uint8, latestAnswer *big.Int, latestTimestamp time.Time, err error) {
	oldResp := latestTransmissionDetailsResponse{}
	var oldErr error
	oldResp.ConfigDigest, oldResp.Epoch, oldResp.Round, oldResp.LatestAnswer, oldResp.LatestTimestamp, oldErr = m.old.LatestTransmissionDetails(ctx)

	// init the LatestAnswer so that it's not nil if this is the first round
	resp := latestTransmissionDetailsResponse{LatestAnswer: new(big.Int)}
	err = m.chainReader.GetLatestValue(ctx, contractName, "LatestTransmissionDetails", nil, &resp)
	cmpPrint(oldResp, resp, oldErr, err, m.lggr, true)

	oldResp = latestTransmissionDetailsResponse{}
	oldResp.ConfigDigest, oldResp.Epoch, oldResp.Round, oldResp.LatestAnswer, oldResp.LatestTimestamp, oldErr = m.old.LatestTransmissionDetails(ctx)
	cmpPrint(oldResp, resp, oldErr, err, m.lggr, false)

	if err != nil {
		return
	}

	return resp.ConfigDigest, resp.Epoch, resp.Round, resp.LatestAnswer, resp.LatestTimestamp, nil
}

func (m *chainReaderContract) LatestRoundRequested(ctx context.Context, lookback time.Duration) (configDigest ocrtypes.ConfigDigest, epoch uint32, round uint8, err error) {
	var resp latestRoundRequested

	var oldResp latestRoundRequested
	var oldErr error
	oldResp.ConfigDigest, oldResp.Epoch, oldResp.Round, oldErr = m.old.LatestRoundRequested(ctx, lookback)

	err = m.chainReader.GetLatestValue(ctx, contractName, "LatestRoundRequested", map[string]string{}, &resp)
	cmpPrint(oldResp, resp, oldErr, err, m.lggr, true)

	oldResp.ConfigDigest, oldResp.Epoch, oldResp.Round, oldErr = m.old.LatestRoundRequested(ctx, lookback)
	cmpPrint(oldResp, resp, oldErr, err, m.lggr, false)

	if err != nil {
		return
	}

	var dnm latestRoundRequested
	return dnm.ConfigDigest, dnm.Epoch, dnm.Round, nil
}

type wrapper struct {
	rc   median.ReportCodec
	old  median.ReportCodec
	lggr logger.Logger
}

func (w *wrapper) BuildReport(observations []median.ParsedAttributedObservation) (ocrtypes.Report, error) {
	b := make([]byte, 2048) // adjust buffer size to be larger than expected stack
	n := runtime.Stack(b, false)
	s := string(b[:n])
	fmt.Printf("Build report called on wrapper %T\n%s\n", w.rc, s)
	oldResults, oldErr := w.old.BuildReport(observations)
	results, err := w.rc.BuildReport(observations)
	cmpPrint(oldResults, results, oldErr, err, w.lggr, true)
	oldResults, oldErr = w.old.BuildReport(observations)
	cmpPrint(oldResults, results, oldErr, err, w.lggr, false)

	return results, err
}

func (w *wrapper) MedianFromReport(report ocrtypes.Report) (*big.Int, error) {
	fmt.Printf("Median from report called on wrapper %T", w.rc)
	oldResults, oldErr := w.old.MedianFromReport(report)
	results, err := w.rc.MedianFromReport(report)
	cmpPrint(oldResults, results, oldErr, err, w.lggr, true)

	oldResults, oldErr = w.old.MedianFromReport(report)
	cmpPrint(oldResults, results, oldErr, err, w.lggr, false)
	return results, err
}

func (w *wrapper) MaxReportLength(n int) (int, error) {
	fmt.Printf("Max report length called on wrapper %T", w.rc)
	oldResults, oldErr := w.old.MaxReportLength(n)
	results, err := w.rc.MaxReportLength(n)
	cmpPrint(oldResults, results, oldErr, err, w.lggr, true)
	oldResults, oldErr = w.old.MaxReportLength(n)
	cmpPrint(oldResults, results, oldErr, err, w.lggr, false)
	return results, err
}

func cmpPrint[T any](expected, actual T, expectedErr, actualErr error, lggr logger.Logger, expectedBefore bool) {
	b := make([]byte, 2048) // adjust buffer size to be larger than expected stack
	n := runtime.Stack(b, false)
	s := string(b[:n])

	same := true

	opt := cmp.Comparer(func(x, y *big.Int) bool {
		return x.Cmp(y) == 0
	})

	diff := cmp.Diff(expected, actual, opt)
	if diff != "" {
		lggr.Errorf("!!!!!!!!\nobject diff found:\n%s\n%s\n%v\n!!!!!!!!", diff, s, expectedBefore)
		same = false
	}

	if !errors.Is(actualErr, expectedErr) {
		lggr.Errorf("!!!!!!!!\nErr diff found:\n%s\n%s\n%v\n!!!!!!!!\n", diff, s, expectedBefore)
		same = false
	}

	if same {
		lggr.Errorf("!!!!!!!!\nNo diff found\n%v\n!!!!!!!!\n", expectedBefore)
	}
}
