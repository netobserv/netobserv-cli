package packet

import (
	"testing"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	"github.com/stretchr/testify/assert"
)

func TestWriterRejectsInvalidBase64(t *testing.T) {
	writer := &Writer{}
	record := config.GenericMap{"Data": "not-base64"}
	assert.Error(t, writer.Write(&record))
}

func TestWriterIgnoresRecordsWithoutData(t *testing.T) {
	writer := &Writer{}
	record := config.GenericMap{}
	assert.NoError(t, writer.Write(&record))
}

func TestCommentsForSortsFieldsBySection(t *testing.T) {
	collectorformat.ConfigureTable(collectorformat.TableSettings{
		Columns: []*collectorformat.ColumnConfig{
			{ID: "SrcAddr", Name: "Src IP", Field: "SrcAddr"},
			{ID: "DstAddr", Name: "Dst IP", Field: "DstAddr"},
			{ID: "Proto", Name: "L3 Protocol", Field: "Proto"},
		},
	})
	comments := commentsFor(config.GenericMap{
		"DstAddr": "10.0.0.2", "SrcAddr": "10.0.0.1", "Proto": float64(6), "Time": float64(10),
	})
	assert.Len(t, comments, 3)
	assert.Contains(t, comments[0], "Source")
	assert.Contains(t, comments[0], "Src IP")
	assert.Contains(t, comments[1], "Destination")
	assert.Contains(t, comments[1], "Dst IP")
	assert.Contains(t, comments[2], "L3 Protocol")
}

func TestPacketTimestampPrefersFlowStartMilliseconds(t *testing.T) {
	record := config.GenericMap{"Time": float64(10), "TimeFlowStartMs": float64(1704067200123)}
	assert.Equal(t, time.UnixMilli(1704067200123), packetTimestamp(record))

	record = config.GenericMap{"Time": float64(10)}
	assert.Equal(t, time.Unix(10, 0), packetTimestamp(record))

}
