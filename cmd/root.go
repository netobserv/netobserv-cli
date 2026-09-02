package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/netobserv/network-observability-cli/internal/pkg/kubernetes"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type captureType string

const (
	Flow   captureType = "Flow"
	Packet captureType = "Packet"
	Metric captureType = "Metric"
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

	mutex = sync.Mutex{}

	totalBytes = int64(0)

	rootCmd = &cobra.Command{
		Use:   "network-observability-cli",
		Short: "network-observability-cli is an interactive Flow and Packet visualizer",
		Long:  `An interactive Flow / PCAP collector and visualization tool`,
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
)

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

// func main() {
func init() {
	cobra.OnInitialize(onInit)
	rootCmd.PersistentFlags().StringVarP(&logLevel, "loglevel", "l", "info", "Log level")
	rootCmd.PersistentFlags().IntVarP(&port, "port", "", 9999, "TCP port to listen")
	rootCmd.PersistentFlags().StringVarP(&filename, "filename", "", "", "Output file name")
	rootCmd.PersistentFlags().StringVarP(&options, "options", "", "", "Options(s)")
	rootCmd.PersistentFlags().DurationVarP(&maxTime, "maxtime", "", 5*time.Minute, "Maximum capture time")
	rootCmd.PersistentFlags().Int64VarP(&maxBytes, "maxbytes", "", 50000000, "Maximum capture bytes")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "netobserv-cli", "Namespace where agent pods are running")
	rootCmd.PersistentFlags().BoolVarP(&useMocks, "mock", "", false, "Use mock")

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		log.Info("Received SIGTERM; cleaning up...")
		stopReceived = true

		os.Exit(0)
	}()

	// flow
	rootCmd.AddCommand(flowCmd)

	// packet
	rootCmd.AddCommand(pktCmd)

	// metrics
	rootCmd.AddCommand(metricCmd)

	// resolve-tls (collector initContainer helper)
	rootCmd.AddCommand(resolveTLSCmd)
}

func onInit() {
	lvl, _ := logrus.ParseLevel(logLevel)
	log.SetLevel(lvl)

	err := LoadConfig()
	if err != nil {
		log.Fatalf("can't load config from yaml: %v", err)
	}

	printBanner()

	log.Infof("Log level: %s\nOption(s): %s", logLevel, options)
	if strings.Contains(options, "background") && !strings.Contains(options, "background=false") {
		isBackground = true
		log.Infof("Running in background mode")
	}
	showKernelVersion()

	if useMocks {
		log.Info("Using mocks...")
		go mockForever()
	}
}

func printBanner() {
	fmt.Print(`
------------------------------------------------------------------------
         _  _     _       _                       ___ _    ___
        | \| |___| |_ ___| |__ ___ ___ _ ___ __  / __| |  |_ _|
        | .' / -_)  _/ _ \ '_ (_-</ -_) '_\ V / | (__| |__ | | 
        |_|\_\___|\__\___/_.__/__/\___|_|  \_/   \___|____|___|

------------------------------------------------------------------------
`)
}

func showKernelVersion() {
	output, err := exec.Command("uname", "-r").Output()
	if err != nil {
		log.Errorf("Can't get kernel version: %v", err)
	}
	if len(output) == 0 {
		log.Infof("Kernel version not found")
	} else {
		log.Infof("Kernel version: %s", strings.TrimSpace(string(output)))
	}
}

func onLimitReached() bool {
	shouldExit := false
	if !captureEnded {
		captureEnded = true
		log.Trace("Capture ended")
		if app != nil && errAdvancedDisplay == nil {
			app.Stop()
		}
		if isBackground {
			err := kubernetes.DeleteDaemonSet(context.Background(), namespace)
			if err != nil {
				log.Error(err)
			}
			fmt.Print(`Thank you for using...`)
			printBanner()

			if capture == "Metric" {
				fmt.Print(`

  - Open NetObserv / On Demand dashboard to see generated metrics

	- Once finished, remove everything using 'oc netobserv cleanup'

                                                      See you soon !
																											
																											
		`)
			} else {
				fmt.Print(`

	- Download the generated output using 'oc netobserv copy' command

	- Once finished, clean the collector pod using 'oc netobserv cleanup'

                                                      See you soon !
																											
																											
		`)
			}

		} else {
			shouldExit = true
		}
	}

	return shouldExit
}

// Create output file, preventing path traversal
func createOutputFile(kind, filename string) (*os.File, error) {
	base := "./output/" + kind + "/"
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Create(filename)
}
