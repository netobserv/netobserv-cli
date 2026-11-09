package collector

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jpillora/sizestr"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/utils"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc/genericmap"
	sharedcapture "github.com/netobserv/netobserv-operator/pkg/capture"
	"github.com/spf13/cobra"

	collectorfiles "github.com/netobserv/network-observability-cli/internal/collector/files"
	flowdb "github.com/netobserv/network-observability-cli/internal/collector/flow"
	collectortransport "github.com/netobserv/network-observability-cli/internal/collector/transport"
)

var flowCmd = &cobra.Command{
	Use:   "get-flows",
	Short: "",
	Long:  "",
	Run:   runFlowCapture,
}

type collectorServer interface {
	Close() error
}

var startGRPCCollector = func(port int, records chan<- *genericmap.Flow, options ...grpc.CollectorOption) (collectorServer, error) {
	return grpc.StartCollector(port, records, options...)
}

func runFlowCapture(_ *cobra.Command, _ []string) {
	capture = Flow
	configureTable()
	ui.showCount = defaultFlowShowCount
	if isBackground {
		go backgroundHeartbeat()
		startFlowCollector()
		return
	}
	go startFlowCollector()
	createFlowDisplay()
}

func startFlowCollector() {
	utils.InitExitChannel()
	if len(filename) > 0 {
		log.Debugf("Starting Flow Capture for %s...", filename)
	} else {
		log.Debug("Starting Flow Capture...")
		filename = strings.ReplaceAll(
			currentTime().UTC().Format(time.RFC3339),
			":", "") // get rid of offensive colons
	}

	// Stream flow records directly into a JSON array. The writer keeps the file
	// readable while the capture runs and finalizes the array on return.
	f, err := collectorfiles.CreateCapture("flow", filename+".json")
	if err != nil {
		log.Errorf("Creating output file failed: %v", err)
		return
	}
	defer f.Close()
	flowWriter := sharedcapture.NewJSONWriter(f)
	var outputMu sync.Mutex
	finalizeOutput := func() {
		outputMu.Lock()
		defer outputMu.Unlock()
		if err := flowWriter.Close(); err != nil {
			log.Errorf("Closing flow JSON file failed: %v", err)
		}
	}
	setCaptureOutputFinalizer(finalizeOutput)
	defer setCaptureOutputFinalizer(nil)
	defer func() {
		finalizeOutput()
	}()
	log.Debugf("Created flow logs JSON file: %s", f.Name())

	var limitTimer *time.Timer
	if isBackground {
		limitTimer = time.AfterFunc(maxTime, func() { onLimitReached() })
		defer limitTimer.Stop()
	}

	// Initialize sqlite DB
	dbPath := filepath.Join(filepath.Dir(f.Name()), filename+".db")
	db, err := flowdb.Open(dbPath)
	if err != nil {
		log.Errorf("Creating flow database failed: %v", err)
		return
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Errorf("Closing flow database failed: %v", err)
		}
	}()
	log.Debug("Initialized database")

	flowPackets := make(chan *genericmap.Flow, 100)
	collector, err := startGRPCCollector(port, flowPackets, collectortransport.ServerOptions(collectorCertPath, collectorKeyPath)...)
	if err != nil {
		log.Errorf("StartCollector failed: %v", err.Error())
		return
	}
	log.Debug("Started collector")
	collectorStarted = true

	go func() {
		<-utils.ExitChannel()
		log.Debug("Ending collector")
		collector.Close()
		close(flowPackets)
		log.Debug("Done")
	}()

	log.Debug("Ready ! Waiting for flows...")
	for fp := range flowPackets {
		if captureEnded {
			continue
		}
		if !captureStarted {
			log.Debugf("Received first %d flows", len(flowPackets))
		}

		if stopReceived {
			log.Debug("Stop received")
			return
		}
		// parse and display flow async
		go parseGenericMapAndAppendFlow(fp.GenericMap.Value)

		// Write flows to sqlite DB
		err = flowdb.Insert(db, fp.GenericMap.Value)
		if err != nil {
			log.Error("Error while writing to DB:", err.Error())
		}
		if !captureStarted {
			log.Debug("Wrote flows to DB")
		}

		outputMu.Lock()
		bytes, err := flowWriter.WriteJSON(fp.GenericMap.Value)
		if err == nil {
			err = flowWriter.Flush()
		}
		outputMu.Unlock()
		if err != nil {
			log.Error(err)
			recordOutputError("flow")
			return
		}
		recordOutput("flows", bytes)
		if !captureStarted {
			log.Debug("Wrote flows to json")
		}

		// terminate capture if max bytes reached
		totalBytes += int64(bytes)
		if totalBytes > maxBytes {
			if exit := onLimitReached(); exit {
				log.Infof("Capture reached %s, exiting now...", sizestr.ToString(maxBytes))
				return
			}
		}

		// terminate capture if max time reached
		now := currentTime()
		duration := now.Sub(startupTime)
		if duration > maxTime {
			if exit := onLimitReached(); exit {
				log.Infof("Capture reached %s, exiting now...", maxTime)
				return
			}
		}

		captureStarted = true
	}
	holdManagedCapture()
}

func parseGenericMapAndAppendFlow(bytes []byte) {
	genericMap, err := flowdb.ParseRecord(bytes)
	if err != nil {
		log.Error("Error while parsing json", err)
		return
	}

	if !captureStarted {
		log.Debugf("Parsed genericMap %v", genericMap)
	}
	appendFlow(genericMap)
}

func clearPacketCaptureBuffers() {
	ui.FlowBuffer.ClearPackets()
}

func packetCaptureFlowSnapshot() []config.GenericMap {
	return ui.FlowBuffer.Snapshot(string(capture), plaintextCaptureEnabled())
}

func appendFlow(record config.GenericMap) {
	ui.FlowBuffer.Append(record, string(capture), plaintextCaptureEnabled(), ui.paused)
}
