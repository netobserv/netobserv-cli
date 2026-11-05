package format

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jpillora/sizestr"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/netobserv/netobserv-ebpf-agent/pkg/utils/networkevents"
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

func toColValue(genericMap config.GenericMap, id string, width int) string {
	// convert column id to its field accordingly
	fieldName := toFieldName(id)
	outputStr := ""

	switch id {
	case tableConfig.RawDisplay:
		outputStr = fmt.Sprintf("%v", genericMap)
	case "StartTime", "EndTime":
		if tableConfig.Capture == "Flow" {
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
			outputStr = PlaintextTablePreview(genericMap, width)
		} else {
			outputStr = toValue(genericMap, "PlaintextPreview")
		}
	default:
		// else simply pick field value as text from column name
		outputStr = toValue(genericMap, fieldName)
	}

	return ellipsizeAndPad(outputStr, width)
}
