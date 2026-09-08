package cmd

import (
	"testing"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

const (
	// this fake flow represent various possible values for every features
	sampleFlow = `{
		"AgentIP":"10.0.1.2",
		"Bytes":456,
		"DnsErrno":0,
		"DnsFlags":34176,
		"DnsFlagsResponseCode":"NoError",
		"DnsId":31319,
		"DnsName":"example.com",
		"DnsLatencyMs":1,
		"Dscp":0,
		"DstAddr":"10.129.0.26",
		"DstK8S_HostIP":"10.0.1.2",
		"DstK8S_HostName":"ip-XX-X-X-XX2.ec2.internal",
		"DstK8S_Name":"dst-pod",
		"DstK8S_Namespace":"second-namespace",
		"DstK8S_OwnerName":"my-statefulset",
		"DstK8S_OwnerType":"StatefulSet",
		"DstK8S_Type":"Pod",
		"DstK8S_Zone":"us-west-1a",
		"DstMac":"0A:58:0A:81:00:1A",
		"DstPort":5678,
		"Duplicate":false,
		"Etype":2048,
		"Flags":16,
		"FlowDirection":0,
		"IfDirections":[1],
		"Interfaces":["f18b970c2ce8fdd"],
		"K8S_FlowLayer":"infra",
		"Packets":5,
		"PktDropBytes":32,
		"PktDropLatestDropCause":"SKB_DROP_REASON_TCP_INVALID_SEQUENCE",
		"PktDropLatestFlags":16,
		"PktDropLatestState":"TCP_INVALID_STATE",
		"PktDropPackets":1,
		"NetworkEvents":[{"Feature":"acl","Type":"NetpolNode","Action":"allow","Direction":"Ingress"}],
		"Proto":6,
		"SrcAddr":"10.128.0.29",
		"SrcK8S_HostIP":"10.0.1.1",
		"SrcK8S_HostName":"ip-XX-X-X-XX1.ec2.internal",
		"SrcK8S_Name":"src-pod",
		"SrcK8S_Namespace":"first-namespace",
		"SrcK8S_OwnerName":"my-deployment",
		"SrcK8S_OwnerType":"Deployment",
		"SrcK8S_Type":"Pod",
		"SrcK8S_Zone":"us-east-1d",
		"SrcMac":"0A:58:0A:81:00:01",
		"SrcPort":1234,
		"TimeFlowEndMs":1709742328703,
		"TimeFlowRttNs":10000,
		"TimeFlowStartMs":1709742328660,
		"TimeReceived":1709742330
	}`
)

var (
	originalTime  = currentTime
	simulatedTime = startupTime
)

func TestDefaultArguments(t *testing.T) {
	assert.Equal(t, "info", logLevel)
	assert.Equal(t, 9999, port)
	assert.Empty(t, options)
}

func TestOptionEnabled(t *testing.T) {
	options = "port=443|--enable_openssl"
	assert.True(t, optionEnabled("enable_openssl"))
	assert.False(t, optionEnabled("enable_gotls"))

	options = "enable_openssl=true|port=443"
	assert.True(t, optionEnabled("enable_openssl"))

	options = "enable_openssl=false"
	assert.False(t, optionEnabled("enable_openssl"))

	// Exact key match: a longer option name must not satisfy a shorter query.
	options = "enable_openssl_debug=true"
	assert.False(t, optionEnabled("enable_openssl"))
	assert.True(t, optionEnabled("enable_openssl_debug"))
}

func TestPlaintextCaptureEnabled(t *testing.T) {
	options = "--enable_openssl"
	assert.True(t, plaintextCaptureEnabled())

	options = "enable_gotls=true"
	assert.False(t, plaintextCaptureEnabled())

	options = "port=443"
	assert.False(t, plaintextCaptureEnabled())
}

func setup(t *testing.T) {
	// reset time to startup time
	resetTime()

	capture = Flow
	options = ""

	// clear filters and previous flows
	regexes = []string{}
	lastFlows = []config.GenericMap{}
	clearPacketCaptureBuffers()
	showCount = defaultFlowShowCount
	selectedData = []byte{}
	paused = false

	// clear previous table content
	tableData = &TableData{
		cols:  []string{},
		flows: []config.GenericMap{},
	}

	// load config
	err := LoadConfig()
	assert.Equal(t, nil, err)
}

func resetTime() {
	// set timezone to Paris time for all tests
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		log.Fatal(err)
	}
	time.Local = loc

	// reset all timers
	currentTime = originalTime
	simulatedTime = startupTime
}

func tickTimeAndAddBytes() {
	currentTime = func() time.Time {
		simulatedTime = simulatedTime.Add(1 * time.Second)
		return simulatedTime
	}

	totalBytes++
}
