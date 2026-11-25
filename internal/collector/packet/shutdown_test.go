package packet

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	"github.com/stretchr/testify/assert"
)

func TestUnverifiedAgentTupleCannotAnnotateAnotherConnection(t *testing.T) {
	for _, tupleSource := range []string{"", "proc", "kernel"} {
		t.Run(tupleSource, func(t *testing.T) {
			var output bytes.Buffer
			buf := NewWireBuffer(time.Minute, func(m config.GenericMap) {
				assert.NoError(t, collectorformat.WriteJSONLine(&output, m))
			}, Filters{}, nil)
			stamp := float64(time.Now().UnixMilli())
			plaintext := config.GenericMap{
				"TimeFlowStartMs": stamp, "Direction": "read", "TLSSource": "openssl",
				"RecordType": "plaintext", "TupleSource": tupleSource,
				"SrcAddr": "10.244.1.2", "SrcPort": float64(8443),
				"DstAddr": "10.244.1.3", "DstPort": float64(45008),
				"Plaintext": base64.StdEncoding.EncodeToString([]byte("GET /marker HTTP/1.1\r\n\r\n")),
			}
			wire := config.GenericMap{
				"TimeFlowStartMs": stamp, "SrcAddr": "10.244.1.3", "SrcPort": float64(45008),
				"DstAddr": "10.244.1.2", "DstPort": float64(8443),
			}
			frame := buildTestTCPFrameWithTuple(t, net.ParseIP("10.244.1.3"), net.ParseIP("10.244.1.2"), 45008, 8443, bytes.Repeat([]byte("x"), 128))
			assert.NoError(t, buf.Enqueue(wire, base64.StdEncoding.EncodeToString(frame)))
			HandlePlaintextRecord(plaintext, buf, &output, RecordCallbacks{AppendFlow: func(config.GenericMap) {}, LogError: func(string, error) {}})
			buf.Close()

			var exported config.GenericMap
			assert.NoError(t, json.Unmarshal(bytes.TrimSpace(output.Bytes()), &exported))
			assert.Equal(t, tupleSource == "kernel", exported["PcapAnnotated"])
			if tupleSource != "kernel" {
				assert.NotContains(t, exported, "SrcAddr")
				assert.NotContains(t, exported, "DstPort")
			} else {
				assert.Equal(t, "10.244.1.3", exported["SrcAddr"])
			}
		})
	}
}

func TestUnmatchedKernelReadKeepsNetworkDirection(t *testing.T) {
	m := config.GenericMap{
		"TupleSource": "kernel", "Direction": "read",
		"SrcAddr": "10.244.1.2", "SrcPort": float64(8443), "SrcK8S_Name": "server",
		"DstAddr": "10.244.1.3", "DstPort": float64(45008), "DstK8S_Name": "client",
	}
	PrepareUnmatchedPlaintext(&m)
	assert.Equal(t, "10.244.1.3", m["SrcAddr"])
	assert.Equal(t, float64(45008), m["SrcPort"])
	assert.Equal(t, "client", m["SrcK8S_Name"])
	assert.Equal(t, "server", m["DstK8S_Name"])
}

func TestOutputStopWaitsForCleanup(t *testing.T) {
	output := NewOutput()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	output.SetStop(func() { cancel(); <-done })
	t.Cleanup(output.Clear)
	stopped := make(chan bool, 1)
	go func() { stopped <- output.Stop() }()
	<-ctx.Done()
	select {
	case <-stopped:
		t.Fatal("stop returned before pending output was flushed")
	default:
	}
	close(done)
	assert.True(t, <-stopped)
}
