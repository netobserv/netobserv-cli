package display

import (
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestTableDataRowsAndBounds(t *testing.T) {
	ConfigureTable("Flow", 0)
	data := NewTableData()
	data.SetData([]string{"EndTime", "SrcAddr"}, []config.GenericMap{{
		"TimeFlowEndMs": float64(1704067200000), "SrcAddr": "10.0.0.1",
	}})

	assert.Equal(t, 2, data.GetRowCount())
	assert.Equal(t, 2, data.GetColumnCount())
	assert.Len(t, data.Rows(), 2)
	assert.Equal(t, "invalid row", data.GetCell(-1, 0).Text)
	assert.Equal(t, "invalid col", data.GetCell(0, -1).Text)
}

func TestTableDataEmptyState(t *testing.T) {
	data := NewTableData()
	assert.Nil(t, data.Rows())
	assert.Equal(t, "Initializing...", data.GetCell(0, 0).Text)
}
