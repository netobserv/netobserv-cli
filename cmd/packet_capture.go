package cmd

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
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

	if tlsKeylogPath != "" {
		if err := embedTLSKeylog(ngw, tlsKeylogPath); err != nil {
			log.Warnf("TLS keylog embed failed: %v", err)
		}
		go watchTLSKeylog(ngw, tlsKeylogPath)
	}

	flowPackets := make(chan *genericmap.Flow, 100)
	collector, err := grpc.StartCollector(port, flowPackets)
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
			assignPlaintextPacketID(&genericMap)
			enrichPlaintextForExport(&genericMap)
			genericMap["PcapAnnotated"] = false
			if plaintextLog != nil {
				writePlaintextJSONL(plaintextLog, &genericMap)
			}
			continue
		}

		data, ok := genericMap["Data"]
		if ok {
			go AppendFlow(genericMap.Copy())
			writePacketData(ngw, &genericMap, &data)
		} else {
			go AppendFlow(genericMap)
		}

		totalBytes += int64(len(fp.GenericMap.Value))
		if totalBytes > maxBytes {
			if exit := onLimitReached(); exit {
				log.Infof("Capture reached %s, exiting now...", sizestr.ToString(maxBytes))
				return
			}
		}

		now := currentTime()
		if int(now.Sub(startupTime)) > int(maxTime) {
			if exit := onLimitReached(); exit {
				log.Infof("Capture reached %s, exiting now...", maxTime)
				return
			}
		}

		captureStarted = true
	}
}

func plaintextCaptureEnabled() bool {
	return optionEnabled("enable_openssl")
}

// clearPacketCaptureBuffers is a no-op until wire/TUI correlation lands (NETOBSERV-2859).
func clearPacketCaptureBuffers() {}

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
	ts := time.Unix(int64((*genericMap)["Time"].(float64)), 0)

	b, err := base64.StdEncoding.DecodeString((*data).(string))
	if err != nil {
		log.Error("Error while decoding data", err)
		return
	}
	keys := make([]string, 0, len((*genericMap)))
	for k := range *genericMap {
		if k == "Time" || k == "Data" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	srcComment.WriteString("Source\n")
	dstComment.WriteString("Destination\n")
	commonComment.WriteString("Common\n")
	for _, k := range keys {
		id := toColID(k)
		str := fmt.Sprintf("%s: %v\n", toColName(id, 0), toColValue((*genericMap), id, 0))
		if strings.HasPrefix(k, "Src") {
			srcComment.WriteString(str)
		} else if strings.HasPrefix(k, "Dst") {
			dstComment.WriteString(str)
		} else {
			commonComment.WriteString(str)
		}
	}

	ngwMu.Lock()
	err = ngw.WritePacketWithOptions(gopacket.CaptureInfo{
		Timestamp:     ts,
		Length:        len(b),
		CaptureLength: len(b),
	}, b, pcapgo.NgPacketOptions{
		Comments: []string{
			srcComment.String(),
			dstComment.String(),
			commonComment.String(),
		},
	})
	ngwMu.Unlock()
	if err != nil {
		log.Error("Error while writing packet", err)
		return
	}

	srcComment.Reset()
	dstComment.Reset()
	commonComment.Reset()
}

// ngwMu serializes all NgWriter writes (packets and TLS keylog DSB blocks).
var ngwMu sync.Mutex
var keylogOffset int64

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
