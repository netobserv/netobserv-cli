package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc/genericmap"
	"google.golang.org/protobuf/types/known/anypb"

	pmod "github.com/prometheus/common/model"
)

func sendMock(ch chan struct{}, client genericmap.CollectorClient) {
	gm := config.GenericMap{
		"DstAddr":          fmt.Sprintf("10.0.0.%d", rand.Intn(255)),
		"DstPort":          rand.Intn(9999),
		"Proto":            rand.Intn(255),
		"SrcAddr":          fmt.Sprintf("10.0.0.%d", rand.Intn(255)),
		"SrcPort":          rand.Intn(9999),
		"SrcK8S_Name":      fmt.Sprintf("Pod-%d", rand.Intn(100)),
		"SrcK8S_Namespace": fmt.Sprintf("Namespace-%d", rand.Intn(100)),
		"SrcK8S_Type":      "Pod",
		"TimeFlowEndMs":    time.Now().UnixMilli(),
	}
	value, err := json.Marshal(gm)
	if err != nil {
		log.Error(err)
		return
	}
	_, err = client.Send(context.Background(), &genericmap.Flow{
		GenericMap: &anypb.Any{
			Value: value,
		},
	})
	if err != nil {
		log.Error(err)
		return
	}

	time.Sleep(10 * time.Millisecond)
	ch <- struct{}{}
}

func mockForever() {
	wait := make(chan struct{})
	for !collectorStarted {
		// collector is not involved in metric capture
		// see mock queryRangeMock below
		if capture == Metric {
			return
		}

		log.Info("Waiting for collector to start...")
		time.Sleep(1 * time.Second)
	}

	cc, err := grpc.ConnectClient("127.0.0.1", port, mockClientTLSConfig())
	if err != nil {
		log.Fatal(err)
	}
	for {
		go sendMock(wait, cc.Client())
		<-wait
	}
}

func matrixMock() QueryResponse {
	// delay response
	secs := rand.Intn(3)
	time.Sleep(time.Duration(secs) * time.Second)

	// create a matrix with at least one metric
	samples := []pmod.SampleStream{}

	// fill values in each sample
	for i := range 10 {
		samples = append(samples, pmod.SampleStream{
			Metric: pmod.Metric{
				"test": pmod.LabelValue(fmt.Sprintf("%d", i)),
			},
			Values: []pmod.SamplePair{},
		})

		// rand numbers to display across time
		now := currentTime().UnixNano()
		val := rand.Float64() * 50
		for j := range showCount {
			now -= int64(j) * 1000000
			samples[i].Values = append(samples[i].Values, pmod.SamplePair{
				Timestamp: pmod.Time(now),
				Value:     pmod.SampleValue(val),
			})
			val += -1 + rand.Float64()*2
		}
	}

	// screw up a timestamp in one of the samples to test robustness
	if len(samples) > 5 && len(samples[5].Values) > 0 {
		samples[5].Values[0].Timestamp += 1234
	}

	// screw up some start values in one of the samples to test robustness
	if len(samples) > 7 && len(samples[7].Values) > 0 {
		samples[7].Values = samples[7].Values[showCount-5 : len(samples[7].Values)]
	}

	// screw up an end value in one of the samples to test robustness
	if len(samples) > 9 && len(samples[9].Values) > 0 {
		samples[9].Values = samples[9].Values[0 : len(samples[9].Values)-1]
	}

	return QueryResponse{
		Data: QueryResponseData{
			ResultType: ResultTypeMatrix,
			Result:     Matrix(samples),
		},
	}
}
