package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
)

func runPacketCapture(_ *cobra.Command, _ []string) {
	capture = Packet
	if isBackground {
		go backgroundHearbeat() // show table periodically in background
		startPacketCollector()
	} else {
		go startPacketCollector()
		createFlowDisplay()
	}
}

//nolint:cyclop
func startPacketCollector() {
	if len(filename) > 0 {
		log.Infof("Starting Packet Capture for %s...", filename)
	} else {
		log.Infof("Starting Packet Capture...")
		filename = strings.ReplaceAll(
			currentTime().UTC().Format(time.RFC3339),
			":", "") // get rid of offensive colons
	}

	f, err := createOutputFile("pcap", filename+".pcapng")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	log.Trace("Created pcapng file")

	ngw, err := pcapgo.NewNgWriter(f, layers.LinkTypeEthernet)
	if err != nil {
		log.Error("Error while creating writer", err)
		return
	}
	defer ngw.Flush()
	log.Trace("Wrote pcap section header & interface")

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
		log.Debug("Ending collector")
		close(flowPackets)
		collector.Close()
		log.Debug("Done")
	}()

	log.Trace("Ready ! Waiting for packets...")
	for fp := range flowPackets {
		if !captureStarted {
			log.Debugf("Received first %d packets", len(flowPackets))
		}

		if stopReceived {
			log.Debug("Stop received")
			return
		}

		genericMap := config.GenericMap{}
		err := json.Unmarshal(fp.GenericMap.Value, &genericMap)
		if err != nil {
			log.Error("Error while parsing json", err)
			return
		}
		if !captureStarted {
			log.Debugf("Parsed genericMap %v", genericMap)
		}

		data, ok := genericMap["Data"]
		if ok {
			// display as flow async
			go AppendFlow(genericMap.Copy())

			writePacketData(ngw, &genericMap, &data)
		} else {
			if !captureStarted {
				log.Debug("Data is missing")
			}

			// display as flow async
			go AppendFlow(genericMap)
		}

		// terminate capture if max bytes reached
		totalBytes += int64(len(fp.GenericMap.Value))
		if totalBytes > maxBytes {
			if exit := onLimitReached(); exit {
				log.Infof("Capture reached %s, exiting now...", sizestr.ToString(maxBytes))
				return
			}
		}

		// terminate capture if max time reached
		now := currentTime()
		duration := now.Sub(startupTime)
		if int(duration) > int(maxTime) {
			if exit := onLimitReached(); exit {
				log.Infof("Capture reached %s, exiting now...", maxTime)
				return
			}
		}

		captureStarted = true
	}
}

func writePacketData(ngw *pcapgo.NgWriter, genericMap *config.GenericMap, data *interface{}) {
	// Get capture timestamp
	ts := time.Unix(int64((*genericMap)["Time"].(float64)), 0)

	// Decode b64 encoded data
	b, err := base64.StdEncoding.DecodeString((*data).(string))
	if err != nil {
		log.Error("Error while decoding data", err)
		return
	}
	// sort generic map keys to keep comments ordered
	keys := make([]string, 0, len((*genericMap)))
	for k := range *genericMap {
		// ignore time field
		if k == "Time" || k == "Data" {
			continue
		}
		keys = append(keys, k)

	}
	sort.Strings(keys)

	// generate comments per category
	srcComment.WriteString("Source\n")
	dstComment.WriteString("Destination\n")
	commonComment.WriteString("Common\n")
	for _, k := range keys {
		id := toColID(k)
		// add name and value without truncating text
		str := fmt.Sprintf("%s: %v\n", toColName(id, 0), toColValue((*genericMap), id, 0))
		if strings.HasPrefix(k, "Src") {
			srcComment.WriteString(str)
		} else if strings.HasPrefix(k, "Dst") {
			dstComment.WriteString(str)
		} else {
			commonComment.WriteString(str)
		}
	}

	// write enriched data as interface
	if err := ngw.WritePacketWithOptions(gopacket.CaptureInfo{
		Timestamp:     ts,
		Length:        len(b),
		CaptureLength: len(b),
	}, b, pcapgo.NgPacketOptions{
		Comments: []string{
			srcComment.String(),
			dstComment.String(),
			commonComment.String(),
		},
	}); err != nil {
		log.Error("Error while writing packet", err)
		return
	}

	srcComment.Reset()
	dstComment.Reset()
	commonComment.Reset()
}
