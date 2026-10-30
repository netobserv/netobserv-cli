package collector

import (
	"context"
	"time"

	collectormetrics "github.com/netobserv/network-observability-cli/internal/collector/metrics"
	"github.com/prometheus/client_golang/api"
	"github.com/spf13/cobra"
)

var metricCmd = &cobra.Command{
	Use:   "get-metrics",
	Short: "",
	Long:  "",
	Run:   runMetricCapture,
}

var (
	client           *api.Client
	selectedDuration = 0
	durations        = []string{"5m", "10m", "30m", "1h", "6h"}
	metricCounts     = map[string]int{
		"5m":  60,
		"10m": 70,
		"30m": 80,
		"1h":  90,
		"6h":  100,
	}
)

func runMetricCapture(c *cobra.Command, _ []string) {
	capture = Metric
	updateGraphs(false)
	if isBackground {
		startMetricCollector(c.Context())
		return
	}
	go startMetricCollector(c.Context())
	createMetricDisplay()
}

func startMetricCollector(ctx context.Context) {
	cl, err := collectormetrics.NewClient(
		time.Duration(30*time.Second),
		false,
		"/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt",
		"/var/run/secrets/kubernetes.io/serviceaccount/token",
		"https://thanos-querier.openshift-monitoring.svc:9091/",
	)
	if err != nil {
		log.Errorf("Error creating client: %v", err.Error())
		log.Fatal(err)
	}

	// save client to be able to call queries from display
	client = &cl
	log.Debug("Created client")

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	log.Trace("Ready ! Querying metrics...")
	for ; true; <-ticker.C {
		if stopReceived {
			log.Debug("Stop received")
			return
		}

		// run query on tick
		queryGraphs(ctx, cl)

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

func queryGraphs(ctx context.Context, client api.Client) {
	for index := range ui.Graphs {
		if isBackground {
			queryGraph(ctx, client, index) // keep logical order for background mode
		} else {
			go queryGraph(ctx, client, index)
		}
	}
}

func queryGraph(ctx context.Context, client api.Client, index int) {
	query, result := queryProm(ctx, client, ui.Graphs[index].Query.PromQL)
	if ui.App == nil || ui.err != nil {
		// simply print metrics into logs
		log.Print(query.PromQL)
		if result == nil || len(*result) == 0 {
			log.Print("  No result")
		} else {
			for _, stream := range *result {
				log.Printf("  %s", stream.String())
			}
		}
	} else {
		appendMetrics(query, result, index)
	}
}

func queryProm(ctx context.Context, client api.Client, promQL string) (*collectormetrics.Query, *collectormetrics.Matrix) {
	now := currentTime()

	ran, err := time.ParseDuration(durations[selectedDuration])
	if err != nil {
		log.Fatal(err)
	}
	query := collectormetrics.BuildQuery(now, ran, ui.showCount, promQL)
	response, err := collectormetrics.QueryMatrix(ctx, client, &query)
	if err != nil {
		log.Error(err)
		return &query, nil
	}

	matrix := response.Data.Result.(collectormetrics.Matrix)
	return &query, &matrix
}
