package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

// The agent's source endpoint identifies the local TLS process. Exported
// endpoints and Kubernetes metadata must instead follow the matched wire frame.
func TestWirePacketBufferPlaintextDirection(t *testing.T) {
	for _, direction := range []string{"read", "write"} {
		for _, fullTuple := range []bool{false, true} {
			for _, plaintextFirst := range []bool{false, true} {
				name := fmt.Sprintf("%s/fullTuple=%t/plaintextFirst=%t", direction, fullTuple, plaintextFirst)
				t.Run(name, func(t *testing.T) {
					var output bytes.Buffer
					ngw, err := pcapgo.NewNgWriter(&output, layers.LinkTypeEthernet)
					assert.NoError(t, err)
					var finalized []config.GenericMap
					buf := newWirePacketBuffer(ngw, time.Minute, func(m config.GenericMap) {
						finalized = append(finalized, m.Copy())
					}, captureFilters{ports: []uint16{8443}, peerIPs: []net.IP{net.ParseIP("10.128.2.17")}})
					t.Cleanup(buf.Close)
					now := float64(time.Now().UnixMilli())
					server := config.GenericMap{
						"TimeFlowStartMs": now,
						"SrcAddr":         "10.128.2.17", "SrcPort": "8443",
						"DstAddr": "10.128.2.18", "DstPort": "36380",
						"SrcK8S_Name": "openssl-test", "DstK8S_Name": "curl-driver",
						"SrcK8S_Type": "Pod", "DstK8S_Type": "Pod",
					}
					client := config.GenericMap{
						"TimeFlowStartMs": now,
						"SrcAddr":         "10.128.2.18", "SrcPort": "36380",
						"DstAddr": "10.128.2.17", "DstPort": "8443",
						"SrcK8S_Name": "curl-driver", "DstK8S_Name": "openssl-test",
						"SrcK8S_Type": "Pod", "DstK8S_Type": "Pod",
					}
					correct, wrong := server, client
					if direction == "read" {
						correct, wrong = client, server
					}
					pt := config.GenericMap{
						"TimeFlowStartMs": now, "Direction": direction, "TLSSource": "openssl",
						"SrcAddr": "10.128.2.17", "SrcPort": float64(8443),
						"SrcK8S_Name": "openssl-test", "SrcK8S_Zone": "stale-zone",
						"DstK8S_Name": "stale-peer", "PlaintextPreview": "HTTP/1.1 200 OK",
					}
					if fullTuple {
						pt["DstAddr"], pt["DstPort"] = "10.128.2.18", float64(36380)
					}
					enqueue := func(m config.GenericMap) {
						t.Helper()
						tuple, ok := flowTupleFromMap(m)
						assert.True(t, ok)
						frame := buildTestTCPFrameWithTuple(t, net.ParseIP(tuple.srcIP), net.ParseIP(tuple.dstIP),
							tuple.srcPort, tuple.dstPort, bytes.Repeat([]byte("x"), 128))
						assert.NoError(t, buf.Enqueue(m, base64.StdEncoding.EncodeToString(frame)))
					}
					if plaintextFirst {
						buf.HandlePlaintext(pt, 6)
					}
					enqueue(wrong)
					assert.Empty(t, finalized, "opposite-direction packet must not consume plaintext")
					enqueue(correct)
					if !plaintextFirst {
						buf.HandlePlaintext(pt, 6)
					}
					if !assert.Len(t, finalized, 1) {
						t.FailNow()
					}
					assert.Equal(t, true, finalized[0]["PcapAnnotated"])
					wantTuple, _ := flowTupleFromMap(correct)
					gotTuple, ok := flowTupleFromMap(finalized[0])
					assert.True(t, ok)
					assert.Equal(t, wantTuple, gotTuple)
					for _, key := range []string{"SrcK8S_Name", "DstK8S_Name", "SrcK8S_Type", "DstK8S_Type"} {
						assert.Equal(t, correct[key], finalized[0][key], key)
					}
					assert.NotContains(t, finalized[0], "SrcK8S_Zone")

					buf.flushAll()
					assert.NoError(t, ngw.Flush())
					reader, err := pcapgo.NewNgReader(bytes.NewReader(output.Bytes()), pcapgo.DefaultNgReaderOptions)
					assert.NoError(t, err)
					_, _, wrongOpts, err := reader.ReadPacketDataWithOptions()
					assert.NoError(t, err)
					assert.Nil(t, wrongOpts.PacketID)
					_, _, correctOpts, err := reader.ReadPacketDataWithOptions()
					assert.NoError(t, err)
					if !assert.NotNil(t, correctOpts.PacketID) {
						t.FailNow()
					}
					assert.Equal(t, uint64(6), *correctOpts.PacketID)
					assert.Contains(t, strings.Join(correctOpts.Comments, "\n"), "5-tuple: "+plaintextFiveTupleLine(finalized[0]))
				})
			}
		}
	}
}

func TestAssignPlaintextPacketID(t *testing.T) {
	plaintextPacketID = 0
	m1 := config.GenericMap{"RecordType": "plaintext"}
	id1 := assignPlaintextPacketID(&m1)
	m2 := config.GenericMap{"RecordType": "plaintext"}
	id2 := assignPlaintextPacketID(&m2)
	if id1 != 1 || id2 != 2 {
		t.Fatalf("expected sequential ids 1,2 got %d,%d", id1, id2)
	}
	if m1["PacketID"] != uint64(1) || m2["PacketID"] != uint64(2) {
		t.Fatalf("unexpected PacketID on maps: %v %v", m1["PacketID"], m2["PacketID"])
	}
}

func TestPlaintextAnnotationComment(t *testing.T) {
	comment := plaintextAnnotationComment(config.GenericMap{
		"Direction":        "write",
		"TLSSource":        "openssl",
		"SrcAddr":          "10.0.1.5",
		"DstAddr":          "10.0.2.3",
		"SrcPort":          float64(45678),
		"DstPort":          float64(443),
		"Pid":              float64(1234),
		"PlaintextPreview": "GET /api HTTP/1.1",
	}, 42)
	for _, want := range []string{
		"PacketID: 42",
		"Direction: write",
		"5-tuple: 10.0.1.5:45678 -> 10.0.2.3:443",
		"GET /api HTTP/1.1",
	} {
		if !strings.Contains(comment, want) {
			t.Fatalf("comment missing %q:\n%s", want, comment)
		}
	}
}

func TestPlaintextTimestampUsesMillis(t *testing.T) {
	ts := plaintextTimestamp(config.GenericMap{"TimeFlowStartMs": float64(1_700_000_000_123)})
	if ts.UnixMilli() != 1_700_000_000_123 {
		t.Fatalf("unexpected timestamp %v", ts)
	}
}

func TestMapPortFromGenericParsesGopacketPortNames(t *testing.T) {
	m := config.GenericMap{"SrcPort": "443(https)", "DstPort": "45148"}
	src, ok := mapPortFromGeneric(m, "SrcPort")
	if !ok || src != 443 {
		t.Fatalf("expected 443, got %d ok=%v", src, ok)
	}
	dst, ok := mapPortFromGeneric(m, "DstPort")
	if !ok || dst != 45148 {
		t.Fatalf("expected 45148, got %d ok=%v", dst, ok)
	}
}

func TestScoreStrictRemoteAndLooseMatch(t *testing.T) {
	now := time.Now()
	pkt := &bufferedWirePacket{
		receivedAt: now,
		packetTime: now,
		genericMap: config.GenericMap{
			"SrcAddr": "10.129.0.15", "DstAddr": "194.62.118.28",
			"SrcPort": "443(https)", "DstPort": "47217",
		},
	}
	ptStrict := &pendingPlaintext{
		receivedAt: now.Add(20 * time.Millisecond),
		eventTime:  now.Add(20 * time.Millisecond),
		data: config.GenericMap{
			"SrcAddr": "10.129.0.15", "DstAddr": "194.62.118.28",
			"SrcPort": float64(443), "DstPort": float64(47217),
			"Direction": "write",
		},
	}
	ptRemote := &pendingPlaintext{
		receivedAt: now.Add(20 * time.Millisecond),
		eventTime:  now.Add(20 * time.Millisecond),
		data: config.GenericMap{
			"SrcAddr": "10.129.0.15", "DstAddr": "194.62.118.28",
			"SrcPort": float64(443), "DstPort": float64(99999),
			"Direction": "write",
		},
	}
	ptLoose := &pendingPlaintext{
		receivedAt: now.Add(20 * time.Millisecond),
		eventTime:  now.Add(20 * time.Millisecond),
		data: config.GenericMap{
			"SrcAddr": "10.129.0.15", "DstAddr": "1.2.3.4",
			"SrcPort": float64(443), "DstPort": float64(99999),
			"Direction": "write",
		},
	}

	strict := scorePlaintextWireMatch(pkt, ptStrict, captureFilters{})
	remote := scorePlaintextWireMatch(pkt, ptRemote, captureFilters{})
	loose := scorePlaintextWireMatch(pkt, ptLoose, captureFilters{})
	if strict <= remote || strict <= loose {
		t.Fatalf("expected strict (%d) to beat remote (%d) and loose (%d)", strict, remote, loose)
	}
	if remote <= loose {
		t.Fatalf("expected remote (%d) to beat loose (%d)", remote, loose)
	}
}

func TestScoreUsesReceiveTimeFallback(t *testing.T) {
	wireNow := time.Now()
	pkt := &bufferedWirePacket{
		receivedAt: wireNow,
		packetTime: wireNow.Add(-10 * time.Second),
		genericMap: config.GenericMap{
			"SrcAddr": "10.128.0.16", "DstAddr": "82.67.17.14",
			"SrcPort": "443(https)", "DstPort": "53726",
		},
	}
	pt := &pendingPlaintext{
		receivedAt: wireNow.Add(100 * time.Millisecond),
		eventTime:  wireNow.Add(-10 * time.Second),
		data: config.GenericMap{
			"SrcAddr": "10.128.0.16", "DstAddr": "82.67.17.14",
			"SrcPort": float64(443), "DstPort": float64(53726),
			"Direction": "write",
		},
	}
	if score := scorePlaintextWireMatch(pkt, pt, captureFilters{}); score < scoreStrictMatchBase {
		t.Fatalf("expected receive-time fallback match, got %d", score)
	}
}

func TestWirePacketBufferBidirectionalMatch(t *testing.T) {
	var buf bytes.Buffer
	ngw, err := pcapgo.NewNgWriter(&buf, layers.LinkTypeEthernet)
	if err != nil {
		t.Fatal(err)
	}

	finalized := make([]config.GenericMap, 0)
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, captureFilters{})

	wireTime := time.Now()
	pt := config.GenericMap{
		"RecordType":       "plaintext",
		"TimeFlowStartMs":  float64(wireTime.UnixMilli()),
		"SrcAddr":          "10.129.0.15",
		"DstAddr":          "194.62.118.28",
		"SrcPort":          float64(443),
		"DstPort":          float64(47217),
		"Direction":        "write",
		"PlaintextPreview": "hello",
	}
	wireBuf.HandlePlaintext(pt, 7)
	if len(finalized) != 0 {
		t.Fatal("expected deferred JSONL until match or expiry")
	}

	wireMap := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.129.0.15", "DstAddr": "194.62.118.28",
		"SrcPort": "443(https)", "DstPort": "47217",
	}
	if err := wireBuf.Enqueue(wireMap, "Zm9v"); err != nil {
		t.Fatal(err)
	}

	if len(finalized) != 1 || finalized[0]["PcapAnnotated"] != true {
		t.Fatalf("expected matched plaintext finalized, got %#v", finalized)
	}

	wireBuf.Close()
	_ = ngw.Flush()
	if buf.Len() == 0 {
		t.Fatal("expected flushed pcap data")
	}
}

func TestWirePacketBufferLooseMatchOnWireArrival(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var buf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&buf, layers.LinkTypeEthernet)
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, captureFilters{})

	wireTime := time.Unix(1_700_000_000, 0)
	pt := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.129.0.15",
		"DstAddr":         "82.67.17.14",
		"SrcPort":         float64(443),
		"DstPort":         float64(99999),
		"Direction":       "write",
	}
	wireBuf.HandlePlaintext(pt, 9)

	wireMap := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.129.0.15", "DstAddr": "82.67.17.14",
		"SrcPort": "443(https)", "DstPort": "50230",
	}
	if err := wireBuf.Enqueue(wireMap, "Zm9v"); err != nil {
		t.Fatal(err)
	}
	if len(finalized) != 1 || finalized[0]["PcapAnnotated"] != true {
		t.Fatalf("expected remote/loose match, got %#v", finalized)
	}
	wireBuf.Close()
}

func TestScoreUsesSymmetricReceiveTimeFallback(t *testing.T) {
	wireNow := time.Now()
	pkt := &bufferedWirePacket{
		receivedAt: wireNow.Add(200 * time.Millisecond),
		packetTime: wireNow.Add(-10 * time.Second),
		genericMap: config.GenericMap{
			"SrcAddr": "10.128.0.16", "DstAddr": "82.67.17.14",
			"SrcPort": "443(https)", "DstPort": "53726",
		},
	}
	pt := &pendingPlaintext{
		receivedAt: wireNow,
		eventTime:  wireNow.Add(-10 * time.Second),
		data: config.GenericMap{
			"SrcAddr": "10.128.0.16", "DstAddr": "82.67.17.14",
			"SrcPort": float64(443), "DstPort": float64(53726),
			"Direction": "write",
		},
	}
	if score := scorePlaintextWireMatch(pkt, pt, captureFilters{}); score < scoreStrictMatchBase {
		t.Fatalf("expected symmetric receive-time fallback match, got %d", score)
	}
}

func TestWirePacketBufferDelayedPlaintextExport(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var buf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&buf, layers.LinkTypeEthernet)
	wireBuf := newWirePacketBuffer(ngw, 30*time.Second, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, captureFilters{})

	wireTime := time.Now()
	wireMap := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.128.0.16", "DstAddr": "82.67.17.14",
		"SrcPort": "443(https)", "DstPort": "53726",
	}
	if err := wireBuf.Enqueue(wireMap, "Zm9v"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	pt := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.Add(100 * time.Millisecond).UnixMilli()),
		"SrcAddr":         "10.128.0.16",
		"DstAddr":         "82.67.17.14",
		"SrcPort":         float64(443),
		"DstPort":         float64(53726),
		"Direction":       "write",
	}
	wireBuf.HandlePlaintext(pt, 12)
	if len(finalized) != 1 || finalized[0]["PcapAnnotated"] != true {
		t.Fatalf("expected delayed plaintext to match buffered wire packet, got %#v", finalized)
	}
	wireBuf.Close()
}

func TestWirePacketBufferPortOnlyFilterMatch(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var buf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&buf, layers.LinkTypeEthernet)
	filters := captureFilters{ports: []uint16{8443}}
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, filters)

	wireTime := time.Now()
	pt := config.GenericMap{
		"RecordType":       "plaintext",
		"TimeFlowStartMs":  float64(wireTime.UnixMilli()),
		"Direction":        "write",
		"TLSSource":        "openssl",
		"PlaintextPreview": "GET / HTTP/1.1",
	}
	wireBuf.HandlePlaintext(pt, 3)

	wireMap := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.244.1.9", "DstAddr": "10.244.2.2",
		"SrcPort": "52442", "DstPort": "8443",
	}
	if err := wireBuf.Enqueue(wireMap, "Zm9v"); err != nil {
		t.Fatal(err)
	}
	if len(finalized) != 1 || finalized[0]["PcapAnnotated"] != true {
		t.Fatalf("expected port-only filter match, got %#v", finalized)
	}
	if finalized[0]["DstAddr"] != "10.244.2.2" {
		t.Fatalf("expected wire tuple enrichment, got %#v", finalized[0])
	}
	wireBuf.Close()
}

func TestWirePacketBufferPeerIPOnlyFilterMatch(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var buf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&buf, layers.LinkTypeEthernet)
	filters := captureFilters{peerIPs: []net.IP{net.ParseIP("10.244.2.2")}}
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, filters)

	wireTime := time.Now()
	pt := config.GenericMap{
		"RecordType":      "plaintext",
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"Direction":       "write",
		"TLSSource":       "openssl",
	}
	wireBuf.HandlePlaintext(pt, 4)

	wireMap := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.244.1.9", "DstAddr": "10.244.2.2",
		"SrcPort": "52442", "DstPort": "8443",
	}
	if err := wireBuf.Enqueue(wireMap, "Zm9v"); err != nil {
		t.Fatal(err)
	}
	if len(finalized) != 1 || finalized[0]["PcapAnnotated"] != true {
		t.Fatalf("expected peer-ip-only filter match, got %#v", finalized)
	}
	wireBuf.Close()
}

func TestWirePacketBufferPortOnlyAmbiguousNoMatch(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var buf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&buf, layers.LinkTypeEthernet)
	filters := captureFilters{ports: []uint16{8443}}
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, filters)

	wireTime := time.Now()
	for _, wireMap := range []config.GenericMap{
		{
			"TimeFlowStartMs": float64(wireTime.UnixMilli()),
			"SrcAddr":         "10.244.1.9", "DstAddr": "10.244.2.2",
			"SrcPort": "52442", "DstPort": "8443",
		},
		{
			"TimeFlowStartMs": float64(wireTime.Add(10 * time.Millisecond).UnixMilli()),
			"SrcAddr":         "10.244.1.10", "DstAddr": "10.244.2.3",
			"SrcPort": "52443", "DstPort": "8443",
		},
	} {
		if err := wireBuf.Enqueue(wireMap, "Zm9v"); err != nil {
			t.Fatal(err)
		}
	}

	pt := config.GenericMap{
		"RecordType":      "plaintext",
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"Direction":       "write",
		"TLSSource":       "openssl",
	}
	wireBuf.HandlePlaintext(pt, 5)

	wireBuf.Close()
	if len(finalized) != 1 || finalized[0]["PcapAnnotated"] == true {
		t.Fatalf("expected ambiguous port-only match to skip annotation, got %#v", finalized)
	}
}

func TestApplyWireTupleToPlaintext(t *testing.T) {
	wire := config.GenericMap{
		"SrcAddr": "10.244.2.11", "DstAddr": "10.244.1.9",
		"SrcPort": float64(8443), "DstPort": float64(51234),
	}
	pt := config.GenericMap{"Direction": "write"}
	applyWireTupleToPlaintext(&pt, wire)
	if pt["SrcAddr"] != "10.244.2.11" || pt["DstAddr"] != "10.244.1.9" {
		t.Fatalf("unexpected write tuple: %#v", pt)
	}
	if pt["SrcPort"] != uint16(8443) || pt["DstPort"] != uint16(51234) {
		t.Fatalf("unexpected write ports: %#v", pt)
	}

	ptRead := config.GenericMap{"Direction": "read"}
	applyWireTupleToPlaintext(&ptRead, config.GenericMap{
		"SrcAddr": "10.244.1.9", "DstAddr": "10.244.2.11",
		"SrcPort": float64(51234), "DstPort": float64(8443),
	})
	if ptRead["SrcAddr"] != "10.244.2.11" || ptRead["DstAddr"] != "10.244.1.9" {
		t.Fatalf("unexpected read tuple: %#v", ptRead)
	}
}

func TestScorePrefersTCPPayloadOverHandshake(t *testing.T) {
	now := time.Now()
	src := net.IP{10, 244, 2, 1}
	dst := net.IP{10, 244, 2, 24}
	syn := &bufferedWirePacket{
		receivedAt: now,
		packetTime: now,
		data:       buildTestTCPFrameWithTuple(t, src, dst, 50348, 8443, nil),
		genericMap: config.GenericMap{
			"SrcAddr": "10.244.2.1", "DstAddr": "10.244.2.24",
			"SrcPort": "50348", "DstPort": "8443",
		},
	}
	payload := &bufferedWirePacket{
		receivedAt: now,
		packetTime: now,
		data: buildTestTCPFrameWithTuple(t, src, dst, 50348, 8443,
			bytes.Repeat([]byte("x"), 120)),
		genericMap: config.GenericMap{
			"SrcAddr": "10.244.2.1", "DstAddr": "10.244.2.24",
			"SrcPort": "50348", "DstPort": "8443",
		},
	}
	pt := &pendingPlaintext{
		receivedAt: now.Add(5 * time.Millisecond),
		eventTime:  now.Add(5 * time.Millisecond),
		data: config.GenericMap{
			"SrcAddr": "10.244.2.1", "DstAddr": "10.244.2.24",
			"SrcPort": float64(50348), "DstPort": float64(8443),
			"Direction": "write", "PlaintextLen": float64(110),
		},
	}
	synScore := scorePlaintextWireMatch(syn, pt, captureFilters{})
	payloadScore := scorePlaintextWireMatch(payload, pt, captureFilters{})
	if synScore >= 0 {
		t.Fatalf("expected handshake to be rejected, got score %d", synScore)
	}
	if payloadScore < minAnnotationScore(pt.data, captureFilters{}) {
		t.Fatalf("expected payload score %d to pass annotation threshold", payloadScore)
	}
	if payloadScore <= synScore {
		t.Fatalf("expected payload (%d) to beat handshake (%d)", payloadScore, synScore)
	}
}

func TestPlaintextWirePodCompatibleRejectsGotlsOnOpensslPod(t *testing.T) {
	pkt := &bufferedWirePacket{
		genericMap: config.GenericMap{
			"DstK8S_Type":      "Pod",
			"DstK8S_Name":      "openssl-test-abc",
			"DstK8S_OwnerName": "openssl-test",
			"DstAddr":          "10.244.2.24",
			"DstPort":          "8443",
			"SrcAddr":          "10.244.2.1",
			"SrcPort":          "50348",
		},
	}
	gotls := config.GenericMap{"TLSSource": "gotls"}
	openssl := config.GenericMap{"TLSSource": "openssl"}
	if plaintextWirePodCompatible(pkt, gotls) {
		t.Fatal("expected gotls to be rejected on openssl pod wire")
	}
	if !plaintextWirePodCompatible(pkt, openssl) {
		t.Fatal("expected openssl to be accepted on openssl pod wire")
	}
}

func TestWirePacketBufferOneAnnotationPerWirePacket(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var buf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&buf, layers.LinkTypeEthernet)
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, captureFilters{})

	wireTime := time.Now()
	src := net.IP{10, 129, 0, 15}
	dst := net.IP{194, 62, 118, 28}
	frame := buildTestTCPFrameWithTuple(t, src, dst, 47217, 443, bytes.Repeat([]byte("a"), 64))
	wireMap := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.129.0.15", "DstAddr": "194.62.118.28",
		"SrcPort": "47217", "DstPort": "443(https)",
	}
	if err := wireBuf.Enqueue(wireMap, base64.StdEncoding.EncodeToString(frame)); err != nil {
		t.Fatal(err)
	}

	for i, id := range []uint64{21, 22} {
		pt := config.GenericMap{
			"TimeFlowStartMs": float64(wireTime.Add(time.Duration(i) * time.Millisecond).UnixMilli()),
			"SrcAddr":         "10.129.0.15",
			"DstAddr":         "194.62.118.28",
			"SrcPort":         float64(47217),
			"DstPort":         float64(443),
			"Direction":       "write",
			"PlaintextLen":    float64(32),
		}
		wireBuf.HandlePlaintext(pt, id)
	}

	annotated := 0
	for _, m := range finalized {
		if m["PcapAnnotated"] == true {
			annotated++
		}
	}
	if annotated != 1 {
		t.Fatalf("expected exactly one annotated plaintext, got %d: %#v", annotated, finalized)
	}
	wireBuf.Close()
}

func TestWirePacketBufferPrefersPayloadOverHandshake(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var pcapBuf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&pcapBuf, layers.LinkTypeEthernet)
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, captureFilters{})

	wireTime := time.Now()
	src := net.IP{10, 244, 2, 1}
	dst := net.IP{10, 244, 2, 24}
	synFrame := buildTestTCPFrameWithTuple(t, src, dst, 50348, 8443, nil)
	payloadFrame := buildTestTCPFrameWithTuple(t, src, dst, 50348, 8443, bytes.Repeat([]byte("b"), 128))

	for i, frame := range [][]byte{synFrame, payloadFrame} {
		wireMap := config.GenericMap{
			"TimeFlowStartMs": float64(wireTime.Add(time.Duration(i) * time.Millisecond).UnixMilli()),
			"SrcAddr":         "10.244.2.1", "DstAddr": "10.244.2.24",
			"SrcPort": "50348", "DstPort": "8443",
			"DstK8S_Type": "Pod", "DstK8S_OwnerName": "openssl-test",
		}
		if err := wireBuf.Enqueue(wireMap, base64.StdEncoding.EncodeToString(frame)); err != nil {
			t.Fatal(err)
		}
	}

	pt := config.GenericMap{
		"TimeFlowStartMs":  float64(wireTime.Add(2 * time.Millisecond).UnixMilli()),
		"SrcAddr":          "10.244.2.1",
		"DstAddr":          "10.244.2.24",
		"SrcPort":          float64(50348),
		"DstPort":          float64(8443),
		"Direction":        "write",
		"TLSSource":        "openssl",
		"PlaintextLen":     float64(110),
		"PlaintextPreview": "GET /healthz HTTP/1.1",
	}
	wireBuf.HandlePlaintext(pt, 99)

	if len(finalized) != 1 || finalized[0]["PcapAnnotated"] != true {
		t.Fatalf("expected openssl plaintext to annotate payload frame, got %#v", finalized)
	}
	wireBuf.Close()
}

func TestWirePacketBufferRejectsGotlsOnOpensslPodWire(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var pcapBuf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&pcapBuf, layers.LinkTypeEthernet)
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, captureFilters{})

	wireTime := time.Now()
	src := net.IP{10, 244, 2, 1}
	dst := net.IP{10, 244, 2, 24}
	frame := buildTestTCPFrameWithTuple(t, src, dst, 50348, 8443, bytes.Repeat([]byte("c"), 64))
	wireMap := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.244.2.1", "DstAddr": "10.244.2.24",
		"SrcPort": "50348", "DstPort": "8443",
		"DstK8S_Type":      "Pod",
		"DstK8S_OwnerName": "openssl-test",
	}
	if err := wireBuf.Enqueue(wireMap, base64.StdEncoding.EncodeToString(frame)); err != nil {
		t.Fatal(err)
	}

	pt := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.244.2.1",
		"DstAddr":         "10.244.2.24",
		"SrcPort":         float64(50348),
		"DstPort":         float64(8443),
		"Direction":       "write",
		"TLSSource":       "gotls",
		"PlaintextLen":    float64(30),
	}
	wireBuf.HandlePlaintext(pt, 100)
	wireBuf.Close()

	if len(finalized) != 1 || finalized[0]["PcapAnnotated"] == true {
		t.Fatalf("expected gotls on openssl pod to stay unannotated, got %#v", finalized)
	}
}

func TestWirePacketBufferCopiesK8sFromCorrelatedWire(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var pcapBuf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&pcapBuf, layers.LinkTypeEthernet)
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, captureFilters{ports: []uint16{8443}})

	wireTime := time.Now()
	src := net.IP{10, 244, 2, 1}
	dst := net.IP{10, 244, 2, 24}
	frame := buildTestTCPFrameWithTuple(t, src, dst, 50348, 8443, bytes.Repeat([]byte("d"), 64))
	wireMap := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.244.2.1", "DstAddr": "10.244.2.24",
		"SrcPort": "50348", "DstPort": "8443",
		"DstK8S_Type":      "Pod",
		"DstK8S_Name":      "openssl-test-pod",
		"DstK8S_OwnerName": "openssl-test",
		"DstK8S_Namespace": "openssl-test",
	}
	if err := wireBuf.Enqueue(wireMap, base64.StdEncoding.EncodeToString(frame)); err != nil {
		t.Fatal(err)
	}

	pt := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"Direction":       "write",
		"TLSSource":       "openssl",
		"PlaintextLen":    float64(110),
	}
	wireBuf.HandlePlaintext(pt, 101)
	wireBuf.Close()

	if len(finalized) != 1 || finalized[0]["PcapAnnotated"] != true {
		t.Fatalf("expected correlated plaintext, got %#v", finalized)
	}
	if finalized[0]["DstK8S_Name"] != "openssl-test-pod" {
		t.Fatalf("expected DstK8S_Name from wire enrichment, got %#v", finalized[0]["DstK8S_Name"])
	}
	if finalized[0]["DstAddr"] != "10.244.2.24" {
		t.Fatalf("expected wire tuple overlay, got %#v", finalized[0]["DstAddr"])
	}
}

// TestWirePacketBufferUnmatchedDropsBorrowedWireFields verifies that a plaintext
// record which speculatively borrows fields from a candidate wire packet but is
// never actually correlated is exported in its pristine form (no borrowed K8s
// metadata, original peer preserved) with PcapAnnotated=false.
func TestWirePacketBufferUnmatchedDropsBorrowedWireFields(t *testing.T) {
	finalized := make([]config.GenericMap, 0)
	var pcapBuf bytes.Buffer
	ngw, _ := pcapgo.NewNgWriter(&pcapBuf, layers.LinkTypeEthernet)
	wireBuf := newWirePacketBuffer(ngw, 50*time.Millisecond, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, captureFilters{})

	wireTime := time.Now()
	// Wire packet shares the pod endpoint (loose match) and carries K8s metadata,
	// but lacks a full 5-tuple (no DstPort) so no tuple can be borrowed.
	wireMap := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.244.1.9",
		"SrcPort":         "443",
		"DstAddr":         "10.244.2.2",
		"DstK8S_Type":     "Pod",
		"DstK8S_Name":     "unrelated-pod",
	}
	if err := wireBuf.Enqueue(wireMap, "Zm9v"); err != nil {
		t.Fatal(err)
	}

	// Plaintext peer (9.9.9.9) is absent from the wire packet, so the remote
	// match fails; only a loose pod:port match (score < remote annotation
	// threshold) is possible, so annotation never succeeds.
	pt := config.GenericMap{
		"TimeFlowStartMs": float64(wireTime.UnixMilli()),
		"SrcAddr":         "10.244.1.9",
		"SrcPort":         float64(443),
		"DstAddr":         "9.9.9.9",
		"Direction":       "write",
	}
	wireBuf.HandlePlaintext(pt, 42)
	wireBuf.Close()

	if len(finalized) != 1 {
		t.Fatalf("expected exactly one finalized record, got %#v", finalized)
	}
	if finalized[0]["PcapAnnotated"] != false {
		t.Fatalf("expected uncorrelated record, got PcapAnnotated=%#v", finalized[0]["PcapAnnotated"])
	}
	if _, borrowed := finalized[0]["DstK8S_Name"]; borrowed {
		t.Fatalf("uncorrelated record leaked wire K8s metadata: %#v", finalized[0])
	}
	if finalized[0]["DstAddr"] != "9.9.9.9" {
		t.Fatalf("expected pristine peer preserved, got %#v", finalized[0]["DstAddr"])
	}
}
