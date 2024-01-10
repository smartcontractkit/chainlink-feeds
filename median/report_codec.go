package median

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2plus/types"
)

const typeName = "MedianReport"

func newReportCodec(codec types.Codec) (median.ReportCodec, error) {
	if codec == nil {
		return nil, errors.New("codec cannot be nil")
	}
	return &reportCodec{codec: codec}, nil
}

type reportCodec struct {
	codec types.Codec
}

var _ median.ReportCodec = &reportCodec{}

func (r *reportCodec) BuildReport(observations []median.ParsedAttributedObservation) (ocrtypes.Report, error) {
	values := make([]*big.Int, len(observations))
	for i, o := range observations {
		values[i] = o.Value
	}
	fmt.Printf("values: %v\n", values)
	if len(observations) == 0 {
		return nil, fmt.Errorf("cannot build report from empty attributed observations")
	}

	return r.codec.Encode(context.Background(), aggregate(observations), typeName)
}

func (r *reportCodec) MedianFromReport(report ocrtypes.Report) (*big.Int, error) {
	agg := &aggregatedAttributedObservation{}
	if err := r.codec.Decode(context.Background(), report, agg, typeName); err != nil {
		return nil, err
	}
	observations := make([]*big.Int, len(agg.Observations))
	for i, o := range agg.Observations {
		observations[i] = o
	}
	fmt.Printf("observations: %v\n", observations)
	medianObservation := len(agg.Observations) / 2
	return agg.Observations[medianObservation], nil
}

func (r *reportCodec) MaxReportLength(n int) (int, error) {
	return r.codec.GetMaxDecodingSize(context.Background(), n, typeName)
}
