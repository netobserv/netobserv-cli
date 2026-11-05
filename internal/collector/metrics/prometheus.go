package metrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	pconf "github.com/prometheus/common/config"
	pmod "github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
)

// BuildQuery creates a range query for the requested duration and point count.
func BuildQuery(now time.Time, duration time.Duration, points int, promQL string) Query {
	if points < 1 {
		points = 1
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, now.Location())
	return Query{
		Range: v1.Range{
			Start: end.Add(-duration),
			End:   end,
			Step:  duration / time.Duration(points),
		},
		PromQL: promQL,
	}
}

// MatrixData aligns samples from all series on the longest timestamp range.
// maxSeries limits the number of series that can be rendered safely.
func MatrixData(matrix Matrix, maxSeries int) (labels []string, legends []map[string]string, data [][]float64) {
	if len(matrix) > maxSeries {
		matrix = matrix[:maxSeries]
	}
	if len(matrix) == 0 {
		return nil, nil, [][]float64{}
	}

	legends = make([]map[string]string, len(matrix))
	data = make([][]float64, len(matrix))
	var timestamps []pmod.Time
	maxValuesLen := 0
	for i, stream := range matrix {
		legends[i] = map[string]string{}
		for key, value := range stream.Metric {
			if !slices.Contains(labels, string(key)) {
				labels = append(labels, string(key))
			}
			legends[i][string(key)] = string(value)
		}
		if len(stream.Values) > maxValuesLen {
			maxValuesLen = len(stream.Values)
			timestamps = timestamps[:0]
			for _, value := range stream.Values {
				timestamps = append(timestamps, value.Timestamp)
			}
		}
	}

	if maxValuesLen > 0 {
		for i, stream := range matrix {
			start := 0
			if maxValuesLen > len(stream.Values) {
				if len(stream.Values) == 0 {
					continue
				}
				for j, timestamp := range timestamps {
					if timestamp.Equal(stream.Values[0].Timestamp) || timestamp.After(stream.Values[0].Timestamp) {
						start = j
						break
					}
				}
				if start == 0 {
					continue
				}
			}
			data[i] = make([]float64, maxValuesLen)
			for j, value := range stream.Values {
				if start+j < len(data[i]) {
					data[i][start+j] = float64(value.Value)
				}
			}
		}
	}
	slices.Sort(labels)
	return labels, legends, data
}

var log = logrus.StandardLogger()

var UseMocks bool
var MockMatrix func() QueryResponse

const (
	ResultTypeStream = "streams"
	ResultTypeScalar = "scalar"
	ResultTypeVector = "vector"
	ResultTypeMatrix = "matrix"
)

type ResultType string

type Query struct {
	Range  v1.Range
	PromQL string
}

type QueryResponse struct {
	Status string            `json:"status"`
	Data   QueryResponseData `json:"data"`
}

type QueryResponseData struct {
	ResultType ResultType  `json:"resultType"`
	Result     ResultValue `json:"result"`
	Stats      interface{} `json:"-"`
}

type ResultValue interface {
	Type() ResultType
}

type Matrix []pmod.SampleStream

func (Matrix) Type() ResultType { return ResultTypeMatrix }

func (Matrix) String() string { return ResultTypeMatrix }

func newTransport(timeout time.Duration, skipTLS bool, capath string, userCertPath string, userKeyPath string) *http.Transport {
	transport := &http.Transport{
		DialContext:     (&net.Dialer{Timeout: timeout}).DialContext,
		IdleConnTimeout: timeout,
	}

	if skipTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		log.Debugf("skipping TLS checks. SSL certificate verification is now disabled !")
	} else if capath != "" || userCertPath != "" {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13}

		if capath != "" {
			caCert, err := os.ReadFile(capath)
			if err != nil {
				log.Errorf("Cannot load ca certificate: %v", err)
			} else {
				pool := x509.NewCertPool()
				pool.AppendCertsFromPEM(caCert)
				transport.TLSClientConfig.RootCAs = pool
			}
		}

		if userCertPath != "" {
			cert, err := tls.LoadX509KeyPair(userCertPath, userKeyPath)
			if err != nil {
				log.Errorf("Cannot load user certificate: %v", err)
			} else {
				transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
			}
		}
	}

	return transport
}

func NewClient(timeout time.Duration, skipTLS bool, caPath string, tokenPath string, url string) (api.Client, error) {
	if UseMocks {
		return api.NewClient(api.Config{})
	}

	maybeTLS := newTransport(timeout, skipTLS, caPath, "", "")

	var roundTripper http.RoundTripper
	if tokenPath != "" {
		bytes, err := os.ReadFile(tokenPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse authorization path '%s': %w", tokenPath, err)
		}
		roundTripper = pconf.NewAuthorizationCredentialsRoundTripper("Bearer", pconf.NewInlineSecret(string(bytes)), maybeTLS)
	} else {
		roundTripper = maybeTLS
	}

	return api.NewClient(api.Config{
		Address:      url,
		RoundTripper: roundTripper,
	})
}

func ExecuteQueryRange(ctx context.Context, cl api.Client, q *Query) (pmod.Value, error) {
	log.Debugf("executeQueryRange: %v; promQL=%s", q.Range, q.PromQL)
	v1api := v1.NewAPI(cl)
	result, warnings, _, err := v1api.QueryRange(ctx, q.PromQL, q.Range)
	log.Tracef("Result:\n%v", result)
	if len(warnings) > 0 {
		log.Infof("executeQueryRange warnings: %v", warnings)
	}
	if err != nil {
		log.Tracef("Error:\n%v", err)
		return nil, fmt.Errorf("error from Prometheus query: %w", err)
	}

	return result, nil
}

func QueryMatrix(ctx context.Context, cl api.Client, q *Query) (QueryResponse, error) {
	if UseMocks && MockMatrix != nil {
		return MockMatrix(), nil
	}

	resp, err := ExecuteQueryRange(ctx, cl, q)
	if err != nil {
		log.WithError(err).Error("Error in QueryMatrix")
		return QueryResponse{}, err
	}
	// Transform response
	m, ok := resp.(pmod.Matrix)
	if !ok {
		err := fmt.Errorf("QueryMatrix: wrong return type: %T", resp)
		log.Error(err.Error())
		return QueryResponse{}, err
	}
	var convMatrix Matrix
	for i := range m {
		convMatrix = append(convMatrix, *m[i])
	}
	qr := QueryResponse{
		Data: QueryResponseData{
			ResultType: ResultTypeMatrix,
			Result:     convMatrix,
		},
	}
	return qr, nil
}
