package collector

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc/genericmap"
	"github.com/stretchr/testify/assert"
)

type testCollectorServer struct{}

func (testCollectorServer) Close() error { return nil }

func TestPacketCaptureIdleDeadlineFlushesFile(t *testing.T) {
	t.Chdir(t.TempDir())
	oldPort, oldFilename, oldMaxTime, oldStartup, oldEnded, oldStart := port, filename, maxTime, startupTime, captureEnded, startGRPCCollector
	t.Cleanup(func() {
		port, filename, maxTime, startupTime, captureEnded, startGRPCCollector = oldPort, oldFilename, oldMaxTime, oldStartup, oldEnded, oldStart
	})
	startGRPCCollector = func(_ int, _ chan<- *genericmap.Flow, _ ...grpc.CollectorOption) (collectorServer, error) {
		return testCollectorServer{}, nil
	}
	port, filename, maxTime, startupTime = 0, "idle", 10*time.Millisecond, time.Now()
	startPacketCollector()
	assert.True(t, captureEnded)
	f, err := os.Open("output/pcap/idle.pcapng")
	assert.NoError(t, err)
	defer f.Close()
	r, err := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
	assert.NoError(t, err, "even an idle capture must flush its PCAP header")
	_, _, err = r.ReadPacketData()
	assert.ErrorIs(t, err, io.EOF)
}
