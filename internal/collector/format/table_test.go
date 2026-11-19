package format_test

import (
	"testing"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"

	collectordisplay "github.com/netobserv/network-observability-cli/internal/collector/display"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
)

func TestFormatTableRows(t *testing.T) {
	previousLocation := time.Local
	time.Local = time.FixedZone("test", int(time.Hour/time.Second))
	t.Cleanup(func() { time.Local = previousLocation })

	displayConfig, err := collectordisplay.LoadConfig()
	assert.NoError(t, err)
	columns := make([]*collectorformat.ColumnConfig, len(displayConfig.Columns))
	for i, column := range displayConfig.Columns {
		columns[i] = &collectorformat.ColumnConfig{
			ID: column.ID, Name: column.Name, Group: column.Group,
			Field: column.Field, Width: column.Width,
		}
	}
	collectorformat.ConfigureTable(collectorformat.TableSettings{
		Columns: columns, RawDisplay: "Raw", ExtraWidth: 5, Capture: "Flow",
	})

	flow := config.GenericMap{
		"TimeFlowEndMs":          float64(1709742328703),
		"SrcK8S_Zone":            "us-east-1d",
		"DstK8S_Zone":            "us-west-1a",
		"SrcK8S_OwnerName":       "my-deployment",
		"DstK8S_OwnerName":       "my-statefulset",
		"SrcK8S_Name":            "src-pod",
		"DstK8S_Name":            "dst-pod",
		"SrcK8S_HostName":        "ip-XX-X-X-XX1.ec2.internal",
		"DstK8S_HostName":        "ip-XX-X-X-XX2.ec2.internal",
		"SrcAddr":                "10.128.0.29",
		"DstAddr":                "10.129.0.26",
		"FlowDirection":          float64(0),
		"Proto":                  float64(6),
		"Dscp":                   float64(0),
		"Bytes":                  float64(456),
		"Packets":                float64(5),
		"PktDropBytes":           float64(32),
		"PktDropPackets":         float64(1),
		"DnsLatencyMs":           float64(1),
		"DnsFlagsResponseCode":   "NoError",
		"TimeFlowRttNs":          float64(10000),
		"PktDropLatestState":     "TCP_INVALID_STATE",
		"PktDropLatestDropCause": "SKB_DROP_REASON_TCP_INVALID_SEQUENCE",
	}

	assert.Equal(t, "17:25:28.703000     ", collectorformat.ToColValue(flow, "EndTime", collectorformat.ToColWidth("EndTime")))
	assert.Equal(t, "us-east-1d          ", collectorformat.ToColValue(flow, "SrcZone", collectorformat.ToColWidth("SrcZone")))
	assert.Equal(t, "us-west-1a          ", collectorformat.ToColValue(flow, "DstZone", collectorformat.ToColWidth("DstZone")))
	assert.Equal(t, "my-deployment       ", collectorformat.ToColValue(flow, "SrcK8S_OwnerName", collectorformat.ToColWidth("SrcK8S_OwnerName")))
	assert.Equal(t, "my-statefulset      ", collectorformat.ToColValue(flow, "DstK8S_OwnerName", collectorformat.ToColWidth("DstK8S_OwnerName")))
	assert.Equal(t, "src-pod             ", collectorformat.ToColValue(flow, "SrcK8S_Name", collectorformat.ToColWidth("SrcK8S_Name")))
	assert.Equal(t, "dst-pod             ", collectorformat.ToColValue(flow, "DstK8S_Name", collectorformat.ToColWidth("DstK8S_Name")))

	assert.Equal(t, "ip-XX-X-X-XX1.ec2.in…", collectorformat.ToColValue(flow, "SrcK8S_HostName", collectorformat.ToColWidth("SrcK8S_HostName")))
	assert.Equal(t, "ip-XX-X-X-XX2.ec2.in…", collectorformat.ToColValue(flow, "DstK8S_HostName", collectorformat.ToColWidth("DstK8S_HostName")))
	assert.Equal(t, "ip-XX-X-X-XX1.ec2.internal", collectorformat.ToColValue(flow, "SrcK8S_HostName", 0))
	assert.Equal(t, "ip-XX-X-X-XX2.ec2.internal", collectorformat.ToColValue(flow, "DstK8S_HostName", 0))

	assert.Equal(t, "10.128.0.29    ", collectorformat.ToColValue(flow, "SrcAddr", collectorformat.ToColWidth("SrcAddr")))
	assert.Equal(t, "10.129.0.26    ", collectorformat.ToColValue(flow, "DstAddr", collectorformat.ToColWidth("DstAddr")))
	assert.Equal(t, "Ingress        ", collectorformat.ToColValue(flow, "FlowDirection", collectorformat.ToColWidth("FlowDirection")))
	assert.Equal(t, "TCP            ", collectorformat.ToColValue(flow, "Proto", collectorformat.ToColWidth("Proto")))
	assert.Equal(t, "Standard       ", collectorformat.ToColValue(flow, "Dscp", collectorformat.ToColWidth("Dscp")))
	assert.Equal(t, "TCP_INVALID…   ", collectorformat.ToColValue(flow, "PktDropLatestState", collectorformat.ToColWidth("PktDropLatestState")))
	assert.Equal(t, "SKB_DROP…      ", collectorformat.ToColValue(flow, "PktDropLatestDropCause", collectorformat.ToColWidth("PktDropLatestDropCause")))

	assert.Equal(t, "456B      ", collectorformat.ToColValue(flow, "Bytes", collectorformat.ToColWidth("Bytes")))
	assert.Equal(t, "5         ", collectorformat.ToColValue(flow, "Packets", collectorformat.ToColWidth("Packets")))
	assert.Equal(t, "32B       ", collectorformat.ToColValue(flow, "PktDropBytes", collectorformat.ToColWidth("PktDropBytes")))
	assert.Equal(t, "1         ", collectorformat.ToColValue(flow, "PktDropPackets", collectorformat.ToColWidth("PktDropPackets")))
	assert.Equal(t, "1ms       ", collectorformat.ToColValue(flow, "DNSLatency", collectorformat.ToColWidth("DNSLatency")))
	assert.Equal(t, "NoError   ", collectorformat.ToColValue(flow, "DNSResponseCode", collectorformat.ToColWidth("DNSResponseCode")))
	assert.Equal(t, "10µs      ", collectorformat.ToColValue(flow, "TimeFlowRttMs", collectorformat.ToColWidth("TimeFlowRttMs")))
}

func TestFormatTableSpecialValuesAndMissingFields(t *testing.T) {
	collectorformat.ConfigureTable(collectorformat.TableSettings{
		Columns: []*collectorformat.ColumnConfig{
			{ID: "Proto", Name: "L3 Protocol", Field: "Proto", Width: 10},
			{ID: "Dscp", Name: "L3 DSCP", Field: "Dscp", Width: 10},
			{ID: "IfDirections", Name: "Interface Dirs", Field: "IfDirections", Width: 15},
		},
	})
	assert.Equal(t, "TCP       ", collectorformat.ToColValue(config.GenericMap{"Proto": float64(6)}, "Proto", 10))
	assert.Equal(t, "Standard  ", collectorformat.ToColValue(config.GenericMap{"Dscp": float64(0)}, "Dscp", 10))
	assert.Equal(t, "Ingress,Egress ", collectorformat.ToColValue(config.GenericMap{
		"IfDirections": []interface{}{float64(0), float64(1)},
	}, "IfDirections", 15))
	assert.Equal(t, "n/a       ", collectorformat.ToColValue(config.GenericMap{}, "Proto", 10))
}
