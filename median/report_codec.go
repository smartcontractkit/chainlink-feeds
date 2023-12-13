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

var _ median.ReportCodec = reportCodec{}

func (r reportCodec) BuildReport(observations []median.ParsedAttributedObservation) (ocrtypes.Report, error) {
	fmt.Printf("!!!!!!!!\nBuildReport\n!!!!!!!!")
	agg := aggregate(observations)
	b, err := r.codec.Encode(context.Background(), agg, typeName)
	fmt.Printf("!!!!!!!!\nBuildReport err: %v\n!!!!!!!!", err)
	return b, err
}

func (r reportCodec) MedianFromReport(report ocrtypes.Report) (*big.Int, error) {
	fmt.Printf("!!!!!!!!\nMedian from report\n!!!!!!!!")
	agg := &aggregatedAttributedObservation{}
	if err := r.codec.Decode(context.Background(), report, agg, typeName); err != nil {
		fmt.Printf("!!!!!!!!\nMedian from report decode err\n%v\n!!!!!!!!", err)
		return nil, err
	}
	medianObservation := len(agg.Observations) / 2
	return agg.Observations[medianObservation], nil
}

func (r reportCodec) MaxReportLength(n int) (int, error) {
	size, err := r.codec.GetMaxDecodingSize(context.Background(), n, typeName)
	fmt.Printf("!!!!!!!!\nMax report length err\n%v\n!!!!!!!!", err)
	return size, err
}
