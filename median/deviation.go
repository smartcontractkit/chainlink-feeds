package median

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
)

var DefaultMultiplier = new(big.Int).SetInt64(1e18)

func NewDeviationFunc(lggr logger.Logger, opts map[string]any) (median.DeviationFunc, error) {
	// Check for type field
	typeVal, ok := opts["type"].(string)
	if !ok {
		return nil, errors.New("missing or invalid 'type' field in deviation function definition")
	}

	switch typeVal {
	case "pendle":
		expiresAt, ok := opts["expiresAt"].(float64) // Assume its a unix TS, it can have fractions of a second
		if !ok {
			return nil, errors.New("missing or invalid 'expiresAt' field in deviation function definition")
		}
		var multiplier *big.Int
		if multiplierStr, ok := opts["multiplier"].(string); ok { // Multiplier could be huge so we use string
			multiplier = new(big.Int)
			if _, ok := multiplier.SetString(multiplierStr, 10); !ok {
				return nil, fmt.Errorf("invalid 'multiplier' field in deviation function definition: %s", multiplierStr)
			}
		} else {
			multiplier = DefaultMultiplier
		}

		return makePendleDeviationFunc(lggr, expiresAt, SystemClock{}, multiplier), nil
	default:
		return nil, fmt.Errorf("unsupported function type in deviation function definition: %s", typeVal)
	}
}

const SecondsInYear = float64(365 * 24 * 60 * 60)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}

// makePendleDeviationFunc makes a pendle-specific deviation func
//
// NOTE: This is non-deterministic if clock.Now() is non-deterministic (the usual case)
// expiresAt expected as float64 number of seconds since epoch
func makePendleDeviationFunc(lggr logger.Logger, expiresAt float64, clock Clock, valMultiplier *big.Int) median.DeviationFunc {
	valMultiplierF := new(big.Float).SetInt(valMultiplier)
	return func(ctx context.Context, thresholdPPB uint64, oldVal, newVal *big.Int) (bool, error) {
		if oldVal == nil || newVal == nil {
			return false, errors.New("oldVal and newVal must be non-nil")
		}

		nowF64 := float64(clock.Now().UnixNano()) / 1e9
		// Convert expirationSeconds to years
		yearsToExpiration := (expiresAt - nowF64) / SecondsInYear

		// Compute absolute difference |oldVal - newVal|
		diff := new(big.Int).Sub(newVal, oldVal)
		diff.Abs(diff) // Take absolute value
		// Convert big.Int to float64 for calculation
		diffFloat := new(big.Float).SetInt(diff)
		// Divide by multiplier
		diffFloat = diffFloat.Quo(diffFloat, valMultiplierF)
		diffF64, _ := diffFloat.Float64()

		// Compute logarithmic threshold
		logThreshold := math.Log(1 + float64(thresholdPPB)/1e9)

		deviates := (diffF64 * yearsToExpiration) > logThreshold

		lggr.Debugw("PendleDeviationFunc", "valMultiplier", valMultiplier.String(), "expiresAt", expiresAt, "thresholdPPB", thresholdPPB, "oldVal", oldVal.String(), "newVal", newVal.String(), "nowF64", nowF64, "yearsToExpiration", yearsToExpiration, "diffF64", diffF64, "logThreshold", logThreshold, "diffF64*yearsToExpiration", diffF64*yearsToExpiration, "deviates", deviates)
		// Return the comparison result
		return deviates, nil
	}
}
