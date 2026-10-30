package collector

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	displayconfig "github.com/netobserv/network-observability-cli/internal/collector/display"
	collectormetrics "github.com/netobserv/network-observability-cli/internal/collector/metrics"
	collectormocks "github.com/netobserv/network-observability-cli/internal/collector/mocks"
	collectorpacket "github.com/netobserv/network-observability-cli/internal/collector/packet"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type captureType string

const (
	Flow   captureType = "Flow"
	Packet captureType = "Packet"
	Metric captureType = "Metric"

	collectorCertPath = "/etc/collector-tls/tls.crt"
	collectorKeyPath  = "/etc/collector-tls/tls.key"
)

var (
	log       = logrus.New()
	logLevel  string
	port      int
	filename  string
	namespace string
	options   string
	maxTime   time.Duration
	maxBytes  int64

	currentTime = time.Now
	startupTime = currentTime()

	signalHandlerOnce sync.Once

	totalBytes = int64(0)

	collectorRootCmd = &cobra.Command{
		Use:   "network-observability-cli",
		Short: "in-pod Flow, Packet and Metrics collector",
		Long:  `Collects and visualizes Flow, Packet and Metrics data inside the collector pod.`,
		Run: func(_ *cobra.Command, _ []string) {
		},
	}

	capture          = Flow
	collectorStarted = false
	captureStarted   = false
	captureEnded     = false
	stopReceived     = false
	useMocks         = false
	isBackground     = false
	packetOutput     = collectorpacket.NewOutput()

	display, enrichment, panels = displayconfig.Defaults()
)

const (
	allOptions              = "All"
	noOptions               = "None"
	rawDisplay              = "Raw"
	standardDisplay         = "Standard"
	pktDropFeature          = "pktDrop"
	dnsFeature              = "dnsTracking"
	rttFeature              = "flowRTT"
	networkEventsDisplay    = "networkEvents"
	pktTranslation          = "packetTranslation"
	udnMapping              = "udnMapping"
	ipSec                   = "ipsec"
	plaintextCaptureFeature = "plaintextCapture"
	defaultDisplayIndex     = 1
	defaultPanelsIndex      = 0
)

func LoadConfig() error {
	if _, err := displayconfig.LoadConfig(); err != nil {
		return err
	}
	displayconfig.ConfigureTable(string(capture), ui.extraWidth)
	return nil
}

func configureTable() {
	displayconfig.ConfigureTable(string(capture), ui.extraWidth)
}

// Execute starts the in-pod collector command. The client-facing plugin has a
// separate command tree in internal/plugin and invokes this binary through
// kubectl/oc exec.
func Execute() error {
	startSignalHandler()
	return collectorRootCmd.Execute()
}

func init() {
	registerCollectorFlags()
	registerCollectorCommands()
	cobra.OnInitialize(initializeCollector)
}

func startSignalHandler() {
	signalHandlerOnce.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM)
		go func() {
			<-c
			log.Info("Received SIGTERM; cleaning up...")
			stopReceived = true

			// Persist any buffered packet capture data before exiting; os.Exit
			// skips deferred flushes, which would otherwise truncate the pcapng.
			if !packetOutput.Stop() {
				writerErr, fileErr := packetOutput.Flush()
				if writerErr != nil {
					log.Errorf("failed to flush pcapng writer on exit: %v", writerErr)
				}
				if fileErr != nil {
					log.Errorf("failed to sync pcapng file on exit: %v", fileErr)
				}
			}

			os.Exit(0)
		}()
	})
}

func initializeCollector() {
	collectormetrics.UseMocks = useMocks
	collectormetrics.MockMatrix = func() collectormetrics.QueryResponse {
		return collectormocks.Matrix(ui.showCount, currentTime())
	}
	lvl, _ := logrus.ParseLevel(logLevel)
	log.SetLevel(lvl)

	err := LoadConfig()
	if err != nil {
		log.Fatalf("can't load config from yaml: %v", err)
	}

	printBanner()

	log.Debugf("Log level: %s\nOption(s): %s", logLevel, options)
	if optionEnabled("background") {
		isBackground = true
		log.Infof("Running in background mode")
	}
	showKernelVersion()

	if useMocks {
		log.Info("Using mocks...")
		go collectormocks.RunFlows(port, collectorCertPath, func() bool { return collectorStarted }, func() bool { return capture == Metric }, func(message string) { log.Info(message) }, func(err error) { log.Error(err) })
	}
}
