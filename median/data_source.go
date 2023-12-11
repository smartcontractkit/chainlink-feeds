package median

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2plus/types"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

type BridgeMetaData struct {
	LatestAnswer *big.Int `json:"latestAnswer"`
	UpdatedAt    *big.Int `json:"updatedAt"` // A unix timestamp
}

func MarshalBridgeMetaData(latestAnswer *big.Int, updatedAt *big.Int) (map[string]interface{}, error) {
	b, err := json.Marshal(&BridgeMetaData{LatestAnswer: latestAnswer, UpdatedAt: updatedAt})
	if err != nil {
		return nil, err
	}
	var mp map[string]interface{}
	err = json.Unmarshal(b, &mp)
	if err != nil {
		return nil, err
	}
	return mp, nil
}

type DataSource struct {
	pipelineRunner types.PipelineRunnerService
	spec           string
	lggr           logger.Logger

	current BridgeMetaData
	mu      sync.RWMutex
}

func (d *DataSource) Observe(ctx context.Context, reportTimestamp ocrtypes.ReportTimestamp) (*big.Int, error) {
	md, err := MarshalBridgeMetaData(d.currentAnswer())
	if err != nil {
		d.lggr.Warnw("unable to attach metadata for run", "err", err)
	}

	// NOTE: job metadata is automatically attached by the pipeline runner service
	vars := types.Vars{
		Vars: map[string]interface{}{
			"jobRun": md,
		},
	}

	results, err := d.pipelineRunner.ExecuteRun(ctx, d.spec, vars, types.Options{})
	if err != nil {
		return nil, err
	}

	finalResults := results.FinalResults()
	if len(finalResults) == 0 {
		return nil, errors.New("pipeline execution failed: not enough results")
	}

	finalResult := finalResults[0]
	if finalResult.Error != nil {
		return nil, fmt.Errorf("pipeline execution failed: %w", finalResult.Error)
	}

	asDecimal, ok := (finalResult.Value).(decimal.Decimal)
	if !ok {
		return nil, errors.New("cannot convert observation to decimal")
	}

	resultAsBigInt := asDecimal.BigInt()
	d.updateAnswer(resultAsBigInt)
	return resultAsBigInt, nil
}

func (d *DataSource) currentAnswer() (*big.Int, *big.Int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.current.LatestAnswer, d.current.UpdatedAt
}

func (d *DataSource) updateAnswer(latestAnswer *big.Int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.current = BridgeMetaData{
		LatestAnswer: latestAnswer,
		UpdatedAt:    big.NewInt(time.Now().Unix()),
	}
}
