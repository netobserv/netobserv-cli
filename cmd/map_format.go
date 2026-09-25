package cmd

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/jpillora/sizestr"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/netobserv/netobserv-ebpf-agent/pkg/utils/networkevents"
)

const (
	emptyText = "n/a"
)

var (
	// dictionnary to shorten long keywords at display
	replacer = strings.NewReplacer(
		"Source", "Src",
		"Destination", "Dst",
		"Direction", "Dir",
		"MultiCluster", "Clusters",
		"Response Code", "RCode",
		"PktDrop", "Drop",
		"DnsTracking", "DNS",
		"FlowRTT", "RTT",
		"Dropped", "Drop",
		"L3 Layer", "L3",
	)
)

func toCount(genericMap config.GenericMap, fieldName string) string {
	v, ok := genericMap[fieldName]
	if ok {
		return sizestr.ToString(int64(v.(float64)))
	}
	return emptyText
}

func toDuration(genericMap config.GenericMap, fieldName string, factor time.Duration) string {
	v, ok := genericMap[fieldName]
	if ok {
		return (time.Duration(int64(v.(float64))) * factor).String()
	}
	return emptyText
}

func directionValue(v float64) string {
	switch v {
	case 0:
		return "Ingress"
	case 1:
		return "Egress"
	case 2:
		return "Inner"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toDirection(genericMap config.GenericMap, fieldName string) string {
	v, ok := genericMap[fieldName]
	if ok {
		if reflect.TypeOf(v).Kind() == reflect.Slice {
			arr := make([]string, len(v.([]interface{})))
			for i, v := range v.([]interface{}) {
				arr[i] = directionValue(v.(float64))
			}
			return strings.Join(arr, ",")
		}
		return directionValue(v.(float64))
	}
	return emptyText
}

//nolint:cyclop
func toProto(genericMap config.GenericMap, fieldName string) string {
	v, ok := genericMap[fieldName]
	if ok {
		switch v.(float64) {
		case 0:
			return "HOPOPT"
		case 1:
			return "ICMP"
		case 2:
			return "IGMP"
		case 3:
			return "GGP"
		case 4:
			return "IPv4"
		case 5:
			return "ST"
		case 6:
			return "TCP"
		case 7:
			return "CBT"
		case 8:
			return "EGP"
		case 9:
			return "IGP"
		case 10:
			return "BBN-RCC-MON"
		case 11:
			return "NVP-II"
		case 12:
			return "PUP"
		case 13:
			return "ARGUS (deprecated)"
		case 14:
			return "EMCON"
		case 15:
			return "XNET"
		case 16:
			return "CHAOS"
		case 17:
			return "UDP"
		case 18:
			return "MUX"
		case 19:
			return "DCN-MEAS"
		case 20:
			return "HMP"
		case 21:
			return "PRM"
		case 22:
			return "XNS-IDP"
		case 23:
			return "TRUNK-1"
		case 24:
			return "TRUNK-2"
		case 25:
			return "LEAF-1"
		case 26:
			return "LEAF-2"
		case 27:
			return "RDP"
		case 28:
			return "IRTP"
		case 29:
			return "ISO-TP4"
		case 30:
			return "NETBLT"
		case 31:
			return "MFE-NSP"
		case 32:
			return "MERIT-INP"
		case 33:
			return "DCCP"
		case 34:
			return "3PC"
		case 35:
			return "IDPR"
		case 36:
			return "XTP"
		case 37:
			return "DDP"
		case 38:
			return "IDPR-CMTP"
		case 39:
			return "TP++"
		case 40:
			return "IL"
		case 41:
			return "IPv6"
		case 42:
			return "SDRP"
		case 43:
			return "IPv6-Route"
		case 44:
			return "IPv6-Frag"
		case 45:
			return "IDRP"
		case 46:
			return "RSVP"
		case 47:
			return "GRE"
		case 48:
			return "DSR"
		case 49:
			return "BNA"
		case 50:
			return "ESP"
		case 51:
			return "AH"
		case 52:
			return "I-NLSP"
		case 53:
			return "SWIPE (deprecated)"
		case 54:
			return "NARP"
		case 55:
			return "MOBILE"
		case 56:
			return "TLSP"
		case 57:
			return "SKIP"
		case 58:
			return "IPv6-ICMP"
		case 59:
			return "IPv6-NoNxt"
		case 60:
			return "IPv6-Opts"
		case 61:
			return "HOST-NETWORK"
		case 62:
			return "CFTP"
		case 63:
			return "LOCAL-NETWORK"
		case 64:
			return "SAT-EXPAK"
		case 65:
			return "KRYPTOLAN"
		case 66:
			return "RVD"
		case 67:
			return "IPPC"
		case 68:
			return "DISTRIBUTED-FS"
		case 69:
			return "SAT-MON"
		case 70:
			return "VISA"
		case 71:
			return "IPCV"
		case 72:
			return "CPNX"
		case 73:
			return "CPHB"
		case 74:
			return "WSN"
		case 75:
			return "PVP"
		case 76:
			return "BR-SAT-MON"
		case 77:
			return "SUN-ND"
		case 78:
			return "WB-MON"
		case 79:
			return "WB-EXPAK"
		case 80:
			return "ISO-IP"
		case 81:
			return "VMTP"
		case 82:
			return "SECURE-VMTP"
		case 83:
			return "VINES"
		case 84:
			return "IPTM"
		case 85:
			return "NSFNET-IGP"
		case 86:
			return "DGP"
		case 87:
			return "TCF"
		case 88:
			return "EIGRP"
		case 89:
			return "OSPFIGP"
		case 90:
			return "Sprite-RPC"
		case 91:
			return "LARP"
		case 92:
			return "MTP"
		case 93:
			return "AX.25"
		case 94:
			return "IPIP"
		case 95:
			return "MICP (deprecated)"
		case 96:
			return "SCC-SP"
		case 97:
			return "ETHERIP"
		case 98:
			return "ENCAP"
		case 99:
			return "PRIVATE-ENCTYPTION"
		case 100:
			return "GMTP"
		case 101:
			return "IFMP"
		case 102:
			return "PNNI"
		case 103:
			return "PIM"
		case 104:
			return "ARIS"
		case 105:
			return "SCPS"
		case 106:
			return "QNX"
		case 107:
			return "A/N"
		case 108:
			return "IPComp"
		case 109:
			return "SNP"
		case 110:
			return "Compaq-Peer"
		case 111:
			return "IPX-in-IP"
		case 112:
			return "VRRP"
		case 113:
			return "PGM"
		case 114:
			return "ZEROHOP"
		case 115:
			return "L2TP"
		case 116:
			return "DDX"
		case 117:
			return "IATP"
		case 118:
			return "STP"
		case 119:
			return "SRP"
		case 120:
			return "UTI"
		case 121:
			return "SMP"
		case 122:
			return "SM (deprecated)"
		case 123:
			return "PTP"
		case 124:
			return "ISIS over IPv4"
		case 125:
			return "FIRE"
		case 126:
			return "CRTP"
		case 127:
			return "CRUDP"
		case 128:
			return "SSCOPMCE"
		case 129:
			return "IPLT"
		case 130:
			return "SPS"
		case 131:
			return "PIPE"
		case 132:
			return "SCTP"
		case 133:
			return "FC"
		case 134:
			return "RSVP-E2E-IGNORE"
		case 135:
			return "Mobility Header"
		case 136:
			return "UDPLite"
		case 137:
			return "MPLS-in-IP"
		case 138:
			return "manet"
		case 139:
			return "HIP"
		case 140:
			return "Shim6"
		case 141:
			return "WESP"
		case 142:
			return "ROHC"
		case 253:
			return "EXPERIMENTAL-253"
		case 254:
			return "EXPERIMENTAL-254"
		case 255:
			return "Reserved"
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	return emptyText
}

func toDSCP(genericMap config.GenericMap, fieldName string) string {
	v, ok := genericMap[fieldName]
	if ok {
		switch v.(float64) {
		case 8:
			return "Low-Priority Data"
		case 0:
			return "Standard"
		case 10:
			return "High-Throughput Data"
		case 16:
			return "OAM"
		case 18:
			return "Low-Latency Data"
		case 24:
			return "Broadcast Video"
		case 26:
			return "Multimedia Streaming"
		case 32:
			return "Real-Time Interactive"
		case 34:
			return "Multimedia Conferencing"
		case 40:
			return "Signaling"
		case 46:
			return "Telephony"
		case 48:
			return "Network Control"
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	return emptyText
}

func toValue(genericMap config.GenericMap, fieldName string) string {
	v, ok := genericMap[fieldName]
	if ok {
		if reflect.TypeOf(v).Kind() == reflect.Slice {
			arr := make([]string, len(v.([]interface{})))
			for i, value := range v.([]interface{}) {
				switch v := value.(type) {
				case string:
					arr[i] = v
					if arr[i] == "" {
						arr[i] = "None"
					}
				default:
					arr[i] = fmt.Sprintf("%v", v)
				}

			}
			return strings.Join(arr, ",")
		}
		return fmt.Sprintf("%v", v)
	}
	return emptyText
}

func toFloat64(genericMap config.GenericMap, fieldName string) float64 {
	v, ok := genericMap[fieldName]
	if ok {
		return v.(float64)
	}
	return 0
}

func toTimeString(genericMap config.GenericMap, fieldName string) string {
	v, ok := genericMap[fieldName]
	if ok {
		return time.UnixMilli(int64(v.(float64))).Format("15:04:05.000000")
	}
	return emptyText
}

func toFieldName(id string) string {
	colIndex := slices.IndexFunc(cfg.Columns, func(c *ColumnConfig) bool { return c.ID == id })
	if colIndex != -1 {
		return cfg.Columns[colIndex].Field
	}
	return ""
}

func ellipsizeAndPad(text string, length int) string {
	if length == 0 {
		return text
	}
	lastPart := length
	currentLen := 0
	truncatedText := text
	for i, r := range text {
		currentLen++
		if currentLen > length {
			truncatedText = text[:lastPart] + "…"
			break
		}

		if unicode.IsSpace(r) || r == '_' {
			lastPart = i
		}
	}
	return fmt.Sprintf("%-*s", length, truncatedText)
}

func toColWidth(id string) int {
	if id == rawDisplay {
		return 0
	}
	colIndex := slices.IndexFunc(cfg.Columns, func(c *ColumnConfig) bool { return c.ID == id })
	width := 6
	if colIndex != -1 {
		width = cfg.Columns[colIndex].Width
	}
	return width + extraWidth
}

func toColID(field string) string {
	colIndex := slices.IndexFunc(cfg.Columns, func(c *ColumnConfig) bool { return c.Field == field })
	if colIndex != -1 {
		return cfg.Columns[colIndex].ID
	}
	return ""
}

func toColName(id string, width int) string {
	name := id
	colIndex := slices.IndexFunc(cfg.Columns, func(c *ColumnConfig) bool { return c.ID == id })
	if colIndex != -1 {
		col := cfg.Columns[colIndex]
		if col.Group != "" && !strings.Contains(col.Name, col.Group) {
			name = fmt.Sprintf("%s %s", col.Group, col.Name)
		} else {
			name = col.Name
		}
	}
	return ellipsizeAndPad(replacer.Replace(name), width)
}

func toMetricName(query string, width int) string {
	name := query
	// remove prefixes
	name = strings.ReplaceAll(name, "on_demand_netobserv_", "")
	// remove times
	name = strings.ReplaceAll(name, "[2m]", "")
	return ellipsizeAndPad(replacer.Replace(name), width)
}

func toColValue(genericMap config.GenericMap, id string, width int) string {
	// convert column id to its field accordingly
	fieldName := toFieldName(id)
	outputStr := ""

	switch id {
	case rawDisplay:
		outputStr = fmt.Sprintf("%v", genericMap)
	case "StartTime", "EndTime":
		if capture == Flow {
			outputStr = toTimeString(genericMap, "TimeFlowEndMs")
		} else {
			outputStr = toTimeString(genericMap, "Time")
		}
	// special cases where autocompletes are involved
	case "FlowDirection", "IfDirections":
		outputStr = toDirection(genericMap, fieldName)
	case "Proto":
		outputStr = toProto(genericMap, fieldName)
	case "Dscp":
		outputStr = toDSCP(genericMap, fieldName)
	// bytes count
	case "Bytes":
		outputStr = toCount(genericMap, "Bytes")
	case "PktDropBytes":
		outputStr = toCount(genericMap, "PktDropBytes")
	// duration parsing
	case "DNSLatency":
		outputStr = toDuration(genericMap, fieldName, time.Millisecond)
	case "TimeFlowRttMs":
		outputStr = toDuration(genericMap, fieldName, time.Nanosecond)
	case "NetworkEvents":
		events := networkevents.MapToStrings(genericMap)
		outputStr = strings.Join(events, ", ")
	case "RecordType":
		if isPlaintextRecord(genericMap) {
			outputStr = toValue(genericMap, "TLSSource")
			if outputStr == "" {
				outputStr = toValue(genericMap, "RecordType")
			}
		} else {
			outputStr = toValue(genericMap, fieldName)
			if outputStr == "" {
				outputStr = toValue(genericMap, "RecordType")
			}
		}
	case "Direction":
		outputStr = toValue(genericMap, "Direction")
	case "PlaintextPreview":
		if isPlaintextRecord(genericMap) {
			outputStr = plaintextTablePreview(genericMap, width)
		} else {
			outputStr = toValue(genericMap, "PlaintextPreview")
		}
	default:
		// else simply pick field value as text from column name
		outputStr = toValue(genericMap, fieldName)
	}

	return ellipsizeAndPad(outputStr, width)
}
