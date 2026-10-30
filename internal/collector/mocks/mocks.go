package mocks

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc/genericmap"
	"github.com/netobserv/network-observability-cli/internal/collector/metrics"
	collectortransport "github.com/netobserv/network-observability-cli/internal/collector/transport"
	pmod "github.com/prometheus/common/model"
	"google.golang.org/protobuf/types/known/anypb"
)

// RunFlows continuously sends synthetic flows to the collector once it is ready.
// Callbacks keep this utility independent from the collector's global state and logger.
func RunFlows(port int, certPath string, started func() bool, metricCapture func() bool, info func(string), reportError func(error)) {
	wait := make(chan struct{})
	for !started() {
		if metricCapture() {
			return
		}
		info("Waiting for collector to start...")
		time.Sleep(time.Second)
	}

	client, err := grpc.ConnectClient("127.0.0.1", port, collectortransport.MockClientConfig(certPath))
	if err != nil {
		reportError(err)
		return
	}
	for {
		go sendFlow(wait, client.Client(), reportError)
		<-wait
	}
}

func sendFlow(done chan<- struct{}, client genericmap.CollectorClient, reportError func(error)) {
	if err := SendFlow(context.Background(), client); err != nil {
		reportError(err)
		return
	}
	time.Sleep(10 * time.Millisecond)
	done <- struct{}{}
}

func SendFlow(ctx context.Context, client genericmap.CollectorClient) error {
	record := config.GenericMap{
		"DstAddr": fmt.Sprintf("10.0.0.%d", rand.Intn(255)), "DstPort": rand.Intn(9999), "Proto": rand.Intn(255),
		"SrcAddr": fmt.Sprintf("10.0.0.%d", rand.Intn(255)), "SrcPort": rand.Intn(9999),
		"SrcK8S_Name": fmt.Sprintf("Pod-%d", rand.Intn(100)), "SrcK8S_Namespace": fmt.Sprintf("Namespace-%d", rand.Intn(100)),
		"SrcK8S_Type": "Pod", "TimeFlowEndMs": time.Now().UnixMilli(),
	}
	value, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = client.Send(ctx, &genericmap.Flow{GenericMap: &anypb.Any{Value: value}})
	return err
}

func Matrix(showCount int, now time.Time) metrics.QueryResponse {
	time.Sleep(time.Duration(rand.Intn(3)) * time.Second)
	samples := make([]pmod.SampleStream, 0, 10)
	for i := range 10 {
		samples = append(samples, pmod.SampleStream{Metric: pmod.Metric{"test": pmod.LabelValue(fmt.Sprintf("%d", i))}, Values: []pmod.SamplePair{}})
		stamp, value := now.UnixNano(), rand.Float64()*50
		for j := 0; j < showCount; j++ {
			stamp -= int64(j) * 1000000
			samples[i].Values = append(samples[i].Values, pmod.SamplePair{Timestamp: pmod.Time(stamp), Value: pmod.SampleValue(value)})
			value += -1 + rand.Float64()*2
		}
	}
	if len(samples) > 5 && len(samples[5].Values) > 0 {
		samples[5].Values[0].Timestamp += 1234
	}
	if len(samples) > 7 && len(samples[7].Values) >= 5 {
		samples[7].Values = samples[7].Values[showCount-5:]
	}
	if len(samples) > 9 && len(samples[9].Values) > 0 {
		samples[9].Values = samples[9].Values[:len(samples[9].Values)-1]
	}
	return metrics.QueryResponse{Data: metrics.QueryResponseData{ResultType: metrics.ResultTypeMatrix, Result: metrics.Matrix(samples)}}
}
