package cmd

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/jpillora/sizestr"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/utils"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc/genericmap"
	"github.com/spf13/cobra"
)

var pktCmd = &cobra.Command{
	Use:   "get-packets",
	Short: "",
	Long:  "",
	Run:   runPacketCapture,
}

var (
	srcComment    strings.Builder
	dstComment    strings.Builder
	commonComment strings.Builder
	tlsKeylogPath string
)

func init() {
	pktCmd.Flags().StringVar(&tlsKeylogPath, "tls-keylog", "", "Path to TLS key log file (SSLKEYLOGFILE format) for pcapng DSB")
}

func runPacketCapture(_ *cobra.Command, _ []string) {
	capture = Packet
	showCount = defaultFlowShowCount
	keepCount = defaultKeepCount
	clearPacketCaptureBuffers()
	if isBackground {
		go backgroundHearbeat()
		startPacketCollector()
	} else {
		go startPacketCollector()
		createFlowDisplay()
	}
}

func startPacketCollector() {
	if len(filename) > 0 {
		log.Infof("Starting Packet Capture for %s...", filename)
	} else {
		log.Infof("Starting Packet Capture...")
		filename = strings.ReplaceAll(
			currentTime().UTC().Format(time.RFC3339),
			":", "")
	}

	f, err := createOutputFile("pcap", filename+".pcapng")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	var plaintextLog io.WriteCloser
	if plaintextCaptureEnabled() {
		plaintextFile, err := createOutputFile("plaintext", filename+".jsonl")
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

	// Register the writer so a SIGTERM can flush buffered packets before the
	// process exits; without this the last buffered packet(s) are lost and the
	// pcapng file ends with a truncated block ("unexpected EOF" on read).
	setActivePacketWriter(ngw, f)
	defer clearActivePacketWriter()

	var wireBuf *wirePacketBuffer
	if plaintextCaptureEnabled() {
		wireBuf = newWirePacketBuffer(ngw, plaintextCorrelationWindow, func(m config.GenericMap) {
			enrichPlaintextForExport(&m)
			if plaintextLog != nil {
				writePlaintextJSONL(plaintextLog, &m)
			}
		}, parseCaptureFilters())
		defer wireBuf.Close()
	}

	if tlsKeylogPath != "" {
		if err := embedTLSKeylog(ngw, tlsKeylogPath); err != nil {
			log.Warnf("TLS keylog embed failed: %v", err)
		}
		go watchTLSKeylog(ngw, tlsKeylogPath)
	}

	flowPackets := make(chan *genericmap.Flow, 100)
	collector, err := grpc.StartCollector(port, flowPackets, collectorTLSOptions()...)
	if err != nil {
		log.Error("StartCollector failed", err)
		return
	}
	log.Debug("Started collector")
	collectorStarted = true

	go func() {
		<-utils.ExitChannel()
		close(flowPackets)
		collector.Close()
	}()

	for fp := range flowPackets {
		if stopReceived {
			return
		}

		genericMap := config.GenericMap{}
		if err := json.Unmarshal(fp.GenericMap.Value, &genericMap); err != nil {
			log.Error("Error while parsing json", err)
			return
		}

		if isPlaintextRecord(genericMap) {
			handlePlaintextRecord(genericMap, wireBuf, plaintextLog)
			continue
		}

		handleWirePacket(ngw, genericMap, wireBuf)

		totalBytes += int64(len(fp.GenericMap.Value))
		if captureLimitReached() {
			return
		}

		captureStarted = true
	}
}

func handlePlaintextRecord(genericMap config.GenericMap, wireBuf *wirePacketBuffer, plaintextLog io.Writer) {
	id := assignPlaintextPacketID(&genericMap)
	enrichPlaintextForExport(&genericMap)
	go AppendFlow(genericMap.Copy())
	if wireBuf != nil {
		wireBuf.HandlePlaintext(genericMap, id)
		return
	}
	genericMap["PcapAnnotated"] = false
	if plaintextLog != nil {
		writePlaintextJSONL(plaintextLog, &genericMap)
	}
}

func handleWirePacket(ngw *pcapgo.NgWriter, genericMap config.GenericMap, wireBuf *wirePacketBuffer) {
	data, ok := genericMap["Data"]
	if !ok {
		go AppendFlow(genericMap)
		return
	}
	go AppendFlow(genericMap.Copy())
	if wireBuf != nil {
		if err := wireBuf.Enqueue(genericMap, data.(string)); err != nil {
			log.Error("failed to buffer wire packet", err)
		}
		return
	}
	writePacketData(ngw, &genericMap, &data)
}

// captureLimitReached reports (and logs) whether the byte or time budget is exhausted.
func captureLimitReached() bool {
	if totalBytes > maxBytes {
		if exit := onLimitReached(); exit {
			log.Infof("Capture reached %s, exiting now...", sizestr.ToString(maxBytes))
			return true
		}
	}
	now := currentTime()
	if int(now.Sub(startupTime)) > int(maxTime) {
		if exit := onLimitReached(); exit {
			log.Infof("Capture reached %s, exiting now...", maxTime)
			return true
		}
	}
	return false
}

func plaintextCaptureEnabled() bool {
	// Only OpenSSL uprobes are implemented on the eBPF agent side today.
	// GoTLS/kTLS tracking is not wired yet, so their flags do not enable
	// plaintext capture (and wire correlation) in the CLI.
	return optionEnabled("enable_openssl")
}

func isPlaintextRecord(m config.GenericMap) bool {
	rt, ok := m["RecordType"].(string)
	return ok && rt == "plaintext"
}

func writePlaintextJSONL(w io.Writer, m *config.GenericMap) {
	line, err := json.Marshal(m)
	if err != nil {
		log.Error("plaintext json marshal", err)
		return
	}
	if _, err := w.Write(append(line, '\n')); err != nil {
		log.Error("plaintext json write", err)
	}
}

func writePacketData(ngw *pcapgo.NgWriter, genericMap *config.GenericMap, data *interface{}) {
	b, err := base64.StdEncoding.DecodeString((*data).(string))
	if err != nil {
		log.Error("Error while decoding data", err)
		return
	}
	if err := writePacketDataWithOptions(ngw, genericMap, b, nil, nil); err != nil {
		log.Error("Error while writing packet", err)
	}
}

// ngwMu serializes all NgWriter writes (packets and TLS keylog DSB blocks) and
// guards the active writer references below.
var ngwMu sync.Mutex
var keylogOffset int64

// activeNgw / activePcapFile point at the in-flight packet capture output, if
// any, so flushActivePacketWriter can persist buffered data on abrupt exit.
var (
	activeNgw      *pcapgo.NgWriter
	activePcapFile *os.File
)

func setActivePacketWriter(ngw *pcapgo.NgWriter, f *os.File) {
	ngwMu.Lock()
	defer ngwMu.Unlock()
	activeNgw = ngw
	activePcapFile = f
}

func clearActivePacketWriter() {
	ngwMu.Lock()
	defer ngwMu.Unlock()
	activeNgw = nil
	activePcapFile = nil
}

// flushActivePacketWriter flushes any buffered pcapng data to disk. It is safe
// to call when no packet capture is running (no-op) and is used from the
// SIGTERM handler, which exits the process without running deferred flushes.
func flushActivePacketWriter() {
	ngwMu.Lock()
	defer ngwMu.Unlock()
	if activeNgw != nil {
		if err := activeNgw.Flush(); err != nil {
			log.Errorf("failed to flush pcapng writer on exit: %v", err)
		}
	}
	if activePcapFile != nil {
		if err := activePcapFile.Sync(); err != nil {
			log.Errorf("failed to sync pcapng file on exit: %v", err)
		}
	}
}

func embedTLSKeylog(ngw *pcapgo.NgWriter, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return nil
	}
	ngwMu.Lock()
	defer ngwMu.Unlock()
	if err := ngw.WriteDecryptionSecretsBlock(pcapgo.DSB_SECRETS_TYPE_TLS, content); err != nil {
		return err
	}
	keylogOffset = int64(len(content))
	return nil
}

func watchTLSKeylog(ngw *pcapgo.NgWriter, path string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if stopReceived {
			return
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		ngwMu.Lock()
		offset := keylogOffset
		ngwMu.Unlock()
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			continue
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil || len(data) == 0 {
			continue
		}
		ngwMu.Lock()
		if err := ngw.WriteDecryptionSecretsBlock(pcapgo.DSB_SECRETS_TYPE_TLS, data); err != nil {
			log.Warnf("failed to append TLS keylog DSB: %v", err)
		} else {
			keylogOffset += int64(len(data))
		}
		ngwMu.Unlock()
	}
}

// ParseKeylogLines reads NSS key log format lines from a reader.
func ParseKeylogLines(r io.Reader) ([]byte, error) {
	var buf strings.Builder
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return []byte(buf.String()), scanner.Err()
}
