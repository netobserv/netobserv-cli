package collector

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const outputMetricsPort = 9401

var (
	outputMetricsOnce sync.Once
	outputRecords     = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "flowcapture_collector_records_total",
		Help: "Number of records written by the FlowCapture collector.",
	}, []string{"output"})
	outputBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "flowcapture_collector_output_bytes_total",
		Help: "Number of bytes written by the FlowCapture collector.",
	}, []string{"output"})
	plaintextRecords = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "flowcapture_collector_plaintext_records_total",
		Help: "Number of plaintext records written by the FlowCapture collector.",
	})
	plaintextBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "flowcapture_collector_plaintext_bytes_total",
		Help: "Number of plaintext bytes written by the FlowCapture collector.",
	})
	outputErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "flowcapture_collector_errors_total",
		Help: "Number of output errors reported by the FlowCapture collector.",
	}, []string{"component"})
)

func startOutputMetricsServer() {
	if !optionEnabled("managed") {
		return
	}
	outputMetricsOnce.Do(func() {
		for _, collector := range []prometheus.Collector{outputRecords, outputBytes, plaintextRecords, plaintextBytes, outputErrors} {
			prometheus.MustRegister(collector)
		}
		go func() {
			if err := http.ListenAndServe(fmt.Sprintf(":%d", outputMetricsPort), promhttp.Handler()); err != nil {
				log.Errorf("output metrics server stopped: %v", err)
			}
		}()
	})
}

func recordOutput(output string, n int) {
	if n <= 0 {
		return
	}
	outputRecords.WithLabelValues(output).Inc()
	outputBytes.WithLabelValues(output).Add(float64(n))
}

func recordPlaintext(n int) {
	if n <= 0 {
		return
	}
	plaintextRecords.Inc()
	plaintextBytes.Add(float64(n))
}

func recordPlaintextOutput(n int) {
	recordOutput("plaintext", n)
	recordPlaintext(n)
}

func recordOutputError(component string) {
	outputErrors.WithLabelValues(component).Inc()
}

type outputCountingWriter struct {
	writer interface{ Write([]byte) (int, error) }
	count  func(int)
}

func (w outputCountingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if err == nil {
		w.count(n)
	}
	return n, err
}
