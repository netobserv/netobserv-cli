package metrics

import (
	"context"
	"testing"
	"time"

	pmod "github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

func TestBuildQueryRoundsEndToMinuteAndCalculatesStep(t *testing.T) {
	now := time.Date(2026, 10, 14, 15, 42, 37, 0, time.UTC)
	query := BuildQuery(now, 10*time.Minute, 20, "sum(rate(flows[2m]))")

	assert.Equal(t, "sum(rate(flows[2m]))", query.PromQL)
	assert.Equal(t, time.Date(2026, 10, 14, 15, 32, 0, 0, time.UTC), query.Range.Start)
	assert.Equal(t, time.Date(2026, 10, 14, 15, 42, 0, 0, time.UTC), query.Range.End)
	assert.Equal(t, 30*time.Second, query.Range.Step)
}

func TestMatrixDataAlignsShorterSeriesAndLimitsSeries(t *testing.T) {
	matrix := Matrix{
		{Metric: pmod.Metric{"job": "api"}, Values: []pmod.SamplePair{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}}},
		{Metric: pmod.Metric{"job": "worker", "zone": "a"}, Values: []pmod.SamplePair{{Timestamp: 2, Value: 4}}},
		{Metric: pmod.Metric{"job": "ignored"}, Values: []pmod.SamplePair{{Timestamp: 1, Value: 9}}},
	}

	labels, legends, data := MatrixData(matrix, 2)
	assert.Equal(t, []string{"job", "zone"}, labels)
	assert.Equal(t, map[string]string{"job": "api"}, legends[0])
	assert.Equal(t, map[string]string{"job": "worker", "zone": "a"}, legends[1])
	assert.Equal(t, [][]float64{{1, 2}, {0, 4}}, data)
}

func TestMatrixDataEmptyMatrix(t *testing.T) {
	labels, legends, data := MatrixData(nil, 5)
	assert.Nil(t, labels)
	assert.Nil(t, legends)
	assert.Empty(t, data)
}

func TestQueryMatrixUsesMockProvider(t *testing.T) {
	oldUseMocks, oldMock := UseMocks, MockMatrix
	t.Cleanup(func() { UseMocks, MockMatrix = oldUseMocks, oldMock })
	UseMocks = true
	MockMatrix = func() QueryResponse { return QueryResponse{Status: "success"} }

	response, err := QueryMatrix(context.TODO(), nil, &Query{})
	assert.NoError(t, err)
	assert.Equal(t, "success", response.Status)
}

func TestNewClientUsesMockModeWithoutReadingCredentials(t *testing.T) {
	old := UseMocks
	t.Cleanup(func() { UseMocks = old })
	UseMocks = true

	client, err := NewClient(time.Second, false, "/does/not/exist", "/does/not/exist", "http://invalid")
	assert.NoError(t, err)
	assert.NotNil(t, client)
}
