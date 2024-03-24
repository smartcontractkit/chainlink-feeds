package median

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/smartcontractkit/libocr/commontypes"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2plus/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

func TestReportCodec(t *testing.T) {
	anyReports := []median.ParsedAttributedObservation{
		{
			Timestamp:        123,
			Value:            big.NewInt(300),
			JuelsPerFeeCoin:  big.NewInt(100),
			GasPriceSubunits: big.NewInt(0),
			Observer:         0,
		},
		{
			Timestamp:        125,
			Value:            big.NewInt(200),
			JuelsPerFeeCoin:  big.NewInt(110),
			GasPriceSubunits: big.NewInt(1),
			Observer:         1,
		},
		{
			Timestamp:        124,
			Value:            big.NewInt(250),
			JuelsPerFeeCoin:  big.NewInt(90),
			GasPriceSubunits: big.NewInt(2),
			Observer:         2,
		},
	}

	aggReports := aggregatedAttributedObservation{
		Timestamp: 124,
		Observers: [32]commontypes.OracleID{1, 2, 0},
		Observations: []*big.Int{
			big.NewInt(200),
			big.NewInt(250),
			big.NewInt(300),
		},
		JuelsPerFeeCoin: big.NewInt(100),
		GasPriceSubunit: big.NewInt(1),
	}

	anyEncodedReport := []byte{5, 6, 7, 8}

	anyError := errors.New("nope not today")

	t.Run("BuildReport builds the type and delegates to generic codec", func(t *testing.T) {
		rc := reportCodec{
			codec: &testCodec{
				t:        t,
				expected: &aggReports,
				result:   anyEncodedReport,
			},
		}

		encoded, err := rc.BuildReport(tests.Context(t), anyReports)
		require.NoError(t, err)
		assert.Equal(t, types.Report(anyEncodedReport), encoded)
	})

	t.Run("BuildReport returns error if there are no reports", func(t *testing.T) {
		rc := reportCodec{
			codec: &testCodec{
				t:        t,
				expected: &aggReports,
				result:   anyEncodedReport,
			},
		}

		ctx := tests.Context(t)
		_, err := rc.BuildReport(ctx, nil)
		assert.Error(t, err)

		_, err = rc.BuildReport(ctx, []median.ParsedAttributedObservation{})
		assert.Error(t, err)
	})

	t.Run("BuildReport returns error if codec returns error", func(t *testing.T) {
		rc := reportCodec{
			&testCodec{
				t:        t,
				expected: &aggReports,
				result:   anyEncodedReport,
				err:      anyError,
			},
		}

		_, err := rc.BuildReport(tests.Context(t), anyReports)
		assert.Equal(t, anyError, err)
	})

	t.Run("MedianFromReport delegates to codec and gets the median", func(t *testing.T) {
		rc := reportCodec{
			&testCodec{
				t:        t,
				expected: anyEncodedReport,
				result:   aggReports,
			},
		}

		medianVal, err := rc.MedianFromReport(tests.Context(t), anyEncodedReport)
		require.NoError(t, err)
		assert.Equal(t, big.NewInt(250), medianVal)
	})

	t.Run("MedianFromReport returns error if codec returns error", func(t *testing.T) {
		rc := reportCodec{
			&testCodec{
				t:        t,
				expected: anyEncodedReport,
				result:   aggReports,
				err:      anyError,
			},
		}

		_, err := rc.MedianFromReport(tests.Context(t), anyEncodedReport)
		assert.Equal(t, anyError, err)
	})

	anyN := 10
	anyLen := 200
	t.Run("MaxReportLength delegates to codec", func(t *testing.T) {
		rc := reportCodec{
			&testCodec{
				t:        t,
				expected: anyN,
				result:   anyLen,
			},
		}

		length, err := rc.MaxReportLength(tests.Context(t), anyN)
		require.NoError(t, err)
		assert.Equal(t, anyLen, length)
	})

	t.Run("MaxReportLength returns error if codec returns error", func(t *testing.T) {
		rc := reportCodec{&testCodec{
			t:        t,
			expected: 10,
			result:   anyLen,
			err:      anyError,
		},
		}

		_, err := rc.MaxReportLength(tests.Context(t), 10)
		assert.Equal(t, anyError, err)
	})
}

type testCodec struct {
	t        *testing.T
	expected any
	result   any
	err      error
}

func (t *testCodec) Encode(_ context.Context, item any, itemType string) ([]byte, error) {
	assert.Equal(t.t, t.expected, item)
	assert.Equal(t.t, typeName, itemType)
	return t.result.([]byte), t.err
}

func (t *testCodec) GetMaxEncodingSize(_ context.Context, n int, itemType string) (int, error) {
	assert.Equal(t.t, t.expected, n)
	assert.Equal(t.t, typeName, itemType)
	return t.result.(int), t.err
}

func (t *testCodec) Decode(_ context.Context, raw []byte, into any, itemType string) error {
	assert.Equal(t.t, t.expected, raw)
	assert.Equal(t.t, typeName, itemType)
	set := into.(*aggregatedAttributedObservation)
	*set = t.result.(aggregatedAttributedObservation)
	return t.err
}

func (t *testCodec) GetMaxDecodingSize(_ context.Context, n int, itemType string) (int, error) {
	assert.Equal(t.t, t.expected, n)
	assert.Equal(t.t, typeName, itemType)
	return t.result.(int), t.err
}
