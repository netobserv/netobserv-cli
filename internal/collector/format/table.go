package format

import "github.com/netobserv/flowlogs-pipeline/pkg/config"

func isPlaintextRecord(m config.GenericMap) bool {
	recordType, ok := m["RecordType"].(string)
	return ok && recordType == "plaintext"
}

func IsPlaintextRecord(m config.GenericMap) bool { return isPlaintextRecord(m) }

type ColumnConfig struct {
	ID, Name, Group, Field string
	Width                  int
}

type TableSettings struct {
	Columns    []*ColumnConfig
	RawDisplay string
	ExtraWidth int
	Capture    string
}

var tableConfig = TableSettings{RawDisplay: "Raw"}

func ConfigureTable(settings TableSettings) { tableConfig = settings }

const emptyText = "n/a"

// Public table-formatting API used by capture writers and the terminal UI.
func ToColID(field string) string                                 { return toColID(field) }
func ToColName(id string, width int) string                       { return toColName(id, width) }
func ToColValue(m config.GenericMap, id string, width int) string { return toColValue(m, id, width) }
func ToColWidth(id string) int                                    { return toColWidth(id) }
func ToMetricName(query string, width int) string                 { return toMetricName(query, width) }
func ToFieldName(id string) string                                { return toFieldName(id) }
func ToFloat64(m config.GenericMap, field string) float64         { return toFloat64(m, field) }
func EllipsizeAndPad(text string, length int) string              { return ellipsizeAndPad(text, length) }
