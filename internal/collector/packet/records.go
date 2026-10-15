package packet

import (
	"fmt"
	"io"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
)

// RecordCallbacks supplies the collector-specific side effects needed while
// handling packet records without coupling this package to the collector UI.
type RecordCallbacks struct {
	AppendFlow func(config.GenericMap)
	LogError   func(message string, err error)
}

func IsPlaintextRecord(record config.GenericMap) bool {
	kind, ok := record["RecordType"].(string)
	return ok && kind == "plaintext"
}

func HandlePlaintextRecord(record config.GenericMap, wireBuf *WireBuffer, plaintextLog io.Writer, callbacks RecordCallbacks) {
	id := AssignPlaintextPacketID(&record)
	collectorformat.EnrichPlaintextForExport(&record)
	go callbacks.AppendFlow(record.Copy())
	if wireBuf != nil && PlaintextTupleVerified(record) {
		wireBuf.HandlePlaintext(record, id)
		return
	}
	PrepareUnmatchedPlaintext(&record)
	record["PcapAnnotated"] = false
	if plaintextLog != nil {
		writePlaintextJSONL(plaintextLog, &record, callbacks)
	}
}

func HandleWirePacket(packetWriter *Writer, record config.GenericMap, wireBuf *WireBuffer, callbacks RecordCallbacks) {
	data, ok := record["Data"]
	if !ok {
		go callbacks.AppendFlow(record)
		return
	}
	go callbacks.AppendFlow(record.Copy())
	if wireBuf != nil {
		dataString, ok := data.(string)
		if !ok {
			callbacks.LogError("wire packet data is not a string", fmt.Errorf("got %T", data))
			return
		}
		if err := wireBuf.Enqueue(record, dataString); err != nil {
			callbacks.LogError("failed to buffer wire packet", err)
		}
		return
	}
	if err := packetWriter.Write(&record); err != nil {
		callbacks.LogError("error while writing packet", err)
	}
}

func writePlaintextJSONL(writer io.Writer, record *config.GenericMap, callbacks RecordCallbacks) {
	if err := collectorformat.WriteJSONLine(writer, *record); err != nil {
		callbacks.LogError("plaintext json write", err)
	}
}
