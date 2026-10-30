package collector

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/jpillora/sizestr"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/utils"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc/genericmap"
	collectorfiles "github.com/netobserv/network-observability-cli/internal/collector/files"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	collectorpacket "github.com/netobserv/network-observability-cli/internal/collector/packet"
	collectortransport "github.com/netobserv/network-observability-cli/internal/collector/transport"
	"github.com/spf13/cobra"
)

var pktCmd = &cobra.Command{
	Use:   "get-packets",
	Short: "",
	Long:  "",
	Run:   runPacketCapture,
}

var tlsKeylogPath string

func init() {
	pktCmd.Flags().StringVar(&tlsKeylogPath, "tls-keylog", "", "Path to TLS key log file (SSLKEYLOGFILE format) for pcapng DSB")
}

func runPacketCapture(_ *cobra.Command, _ []string) {
	capture = Packet
	configureTable()
	ui.showCount = defaultFlowShowCount
	clearPacketCaptureBuffers()
	if isBackground {
		go backgroundHeartbeat()
		startPacketCollector()
		return
	}
	go startPacketCollector()
	createFlowDisplay()
	packetOutput.Stop()
}

// startPacketCollector coordinates the packet writer, optional plaintext
// correlation, and collector lifecycle. The select below intentionally keeps
// all termination paths together so every path flushes the capture correctly.
func startPacketCollector() { //nolint:cyclop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	completed := false
	// Announce completion only after deferred buffer/file flushes have run.
	defer func() {
		if completed {
			onLimitReached()
		}
	}()

	if len(filename) > 0 {
		log.Debugf("Starting Packet Capture for %s...", filename)
	} else {
		log.Debug("Starting Packet Capture...")
		filename = strings.ReplaceAll(
			currentTime().UTC().Format(time.RFC3339),
			":", "")
	}

	f, err := collectorfiles.CreateCapture("pcap", filename+".pcapng")
	if err != nil {
		log.Error(err)
		return
	}
	defer f.Close()

	var plaintextLog io.WriteCloser
	if plaintextCaptureEnabled() {
		plaintextFile, err := collectorfiles.CreateCapture("plaintext", filename+".jsonl")
		if err != nil {
			log.Error("failed to create plaintext log", err)
		} else {
			plaintextLog = plaintextFile
			defer plaintextLog.Close()
		}
	}

	ngw, err := pcapgo.NewNgWriter(f, layers.LinkTypeEthernet)
	if err != nil {
		log.Error("Error while creating writer", err)
		return
	}
	defer ngw.Flush()
	packetWriter := collectorpacket.NewWriter(ngw, packetOutput.Mutex())

	// Register the writer so a SIGTERM can flush buffered packets before the
	// process exits; without this the last buffered packet(s) are lost and the
	// pcapng file ends with a truncated block ("unexpected EOF" on read).
	packetOutput.SetWriter(ngw, f)
	packetOutput.SetStop(func() { cancel(); <-done })
	defer packetOutput.Clear()

	// NgWriter buffers pcapng blocks. Flush periodically so a managed capture
	// can be copied while it is still running, just like an interactive one.
	// The final deferred Flush still provides the complete, durable file.
	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if writerErr, fileErr := packetOutput.Flush(); writerErr != nil || fileErr != nil {
					if writerErr != nil {
						log.Errorf("failed to flush pcapng writer: %v", writerErr)
					}
					if fileErr != nil {
						log.Errorf("failed to sync pcapng file: %v", fileErr)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-flushDone
	}()

	var wireBuf *collectorpacket.WireBuffer
	if plaintextCaptureEnabled() {
		wireBuf = collectorpacket.NewWireBuffer(collectorpacket.CorrelationWindow, func(m config.GenericMap) {
			collectorformat.EnrichPlaintextForExport(&m)
			if plaintextLog != nil {
				if err := collectorformat.WriteJSONLine(plaintextLog, m); err != nil {
					log.Error("plaintext json write", err)
				}
			}
		}, collectorpacket.ParseFilters(options), func(m *config.GenericMap, data []byte, comments []string, packetID *uint64) error {
			return packetWriter.WriteData(m, data, comments, packetID)
		})
		defer wireBuf.Close()
	}

	if tlsKeylogPath != "" {
		keylog := collectorpacket.NewKeylog(ngw, packetOutput.Mutex())
		if err := keylog.Embed(tlsKeylogPath); err != nil {
			log.Warnf("TLS keylog embed failed: %v", err)
		}
		go keylog.Watch(ctx, tlsKeylogPath)
	}

	flowPackets := make(chan *genericmap.Flow, 100)
	collector, err := startGRPCCollector(port, flowPackets, collectortransport.ServerOptions(collectorCertPath, collectorKeyPath)...)
	if err != nil {
		log.Error("StartCollector failed", err)
		return
	}
	log.Debug("Started collector")
	collectorStarted = true

	defer collector.Close()
	deadline := time.NewTimer(max(time.Duration(0), maxTime-currentTime().Sub(startupTime)))
	defer deadline.Stop()
	exit := utils.ExitChannel()
	for {
		var fp *genericmap.Flow
		select {
		case <-ctx.Done():
			completed = true
			return
		case <-exit:
			completed = true
			return
		case <-deadline.C:
			log.Infof("Capture reached %s, exiting now...", maxTime)
			completed = true
			return
		case fp = <-flowPackets:
		}
		genericMap := config.GenericMap{}
		if err := json.Unmarshal(fp.GenericMap.Value, &genericMap); err != nil {
			log.Error("Error while parsing json", err)
			continue
		}
		handlePacketRecord(packetWriter, genericMap, wireBuf, plaintextLog)
		totalBytes += int64(len(fp.GenericMap.Value))
		if totalBytes > maxBytes {
			log.Infof("Capture reached %s, exiting now...", sizestr.ToString(maxBytes))
			completed = true
			return
		}
		captureStarted = true
	}
}

func handlePacketRecord(packetWriter *collectorpacket.Writer, record config.GenericMap, wireBuf *collectorpacket.WireBuffer, plaintextLog io.Writer) {
	callbacks := collectorpacket.RecordCallbacks{
		AppendFlow: func(record config.GenericMap) { appendFlow(record) },
		LogError:   func(message string, err error) { log.Error(message, err) },
	}
	if collectorpacket.IsPlaintextRecord(record) {
		collectorpacket.HandlePlaintextRecord(record, wireBuf, plaintextLog, callbacks)
		return
	}
	collectorpacket.HandleWirePacket(packetWriter, record, wireBuf, callbacks)
}

func plaintextCaptureEnabled() bool {
	// Only OpenSSL uprobes are implemented on the eBPF agent side today.
	// GoTLS/kTLS tracking is not wired yet, so their flags do not enable
	// plaintext capture (and wire correlation) in the CLI.
	return optionEnabled("enable_openssl")
}
