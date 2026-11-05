package packet

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
)

// Writer serializes flow records into a pcapng stream.
type Writer struct {
	mu  *sync.Mutex
	ngw *pcapgo.NgWriter
}

func NewWriter(ngw *pcapgo.NgWriter, mu *sync.Mutex) *Writer { return &Writer{ngw: ngw, mu: mu} }

func (w *Writer) Write(record *config.GenericMap) error {
	value, ok := (*record)["Data"].(string)
	if !ok {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	return w.WriteData(record, data, nil, nil)
}

func (w *Writer) WriteData(record *config.GenericMap, data []byte, extraComments []string, packetID *uint64) error {
	comments := commentsFor(*record)
	comments = append(comments, extraComments...)
	options := pcapgo.NgPacketOptions{Comments: comments, PacketID: packetID}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ngw.WritePacketWithOptions(gopacket.CaptureInfo{
		Timestamp: packetTimestamp(*record), Length: len(data), CaptureLength: len(data),
	}, data, options)
}

func commentsFor(record config.GenericMap) []string {
	keys := make([]string, 0, len(record))
	for key := range record {
		if key != "Time" && key != "Data" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var source, destination, common strings.Builder
	source.WriteString("Source\n")
	destination.WriteString("Destination\n")
	common.WriteString("Common\n")
	for _, key := range keys {
		id := collectorformat.ToColID(key)
		line := fmt.Sprintf("%s: %v\n", collectorformat.ToColName(id, 0), collectorformat.ToColValue(record, id, 0))
		switch {
		case strings.HasPrefix(key, "Src"):
			source.WriteString(line)
		case strings.HasPrefix(key, "Dst"):
			destination.WriteString(line)
		default:
			common.WriteString(line)
		}
	}
	return []string{source.String(), destination.String(), common.String()}
}

func packetTimestamp(record config.GenericMap) time.Time {
	if value, ok := record["TimeFlowStartMs"].(float64); ok && value > 0 {
		return time.UnixMilli(int64(value))
	}
	if value, ok := record["Time"].(float64); ok {
		return time.Unix(int64(value), 0)
	}
	return time.Now()
}
