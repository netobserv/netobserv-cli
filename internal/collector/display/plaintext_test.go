package display

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddPlaintextColumns(t *testing.T) {
	cfg, err := LoadConfig()
	assert.NoError(t, err)

	columns := AddPlaintextColumns([]string{"EndTime", "SrcAddr"}, cfg, true, true, nil)
	assert.Equal(t, []string{"EndTime", "SrcAddr", "RecordType", "Direction", "PlaintextPreview"}, columns)

	columns = AddPlaintextColumns([]string{"EndTime", "SrcAddr"}, cfg, true, true, []string{"EndTime", "SrcAddr"})
	assert.Equal(t, []string{"EndTime", "SrcAddr"}, columns)

	columns = AddPlaintextColumns([]string{"EndTime"}, cfg, false, true, nil)
	assert.Equal(t, []string{"EndTime"}, columns)
}
