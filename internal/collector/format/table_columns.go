package format

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
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

func toFieldName(id string) string {
	colIndex := slices.IndexFunc(tableConfig.Columns, func(c *ColumnConfig) bool { return c.ID == id })
	if colIndex != -1 {
		return tableConfig.Columns[colIndex].Field
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
	if id == tableConfig.RawDisplay {
		return 0
	}
	colIndex := slices.IndexFunc(tableConfig.Columns, func(c *ColumnConfig) bool { return c.ID == id })
	width := 6
	if colIndex != -1 {
		width = tableConfig.Columns[colIndex].Width
	}
	return width + tableConfig.ExtraWidth
}

func toColID(field string) string {
	colIndex := slices.IndexFunc(tableConfig.Columns, func(c *ColumnConfig) bool { return c.Field == field })
	if colIndex != -1 {
		return tableConfig.Columns[colIndex].ID
	}
	return ""
}

func toColName(id string, width int) string {
	name := id
	colIndex := slices.IndexFunc(tableConfig.Columns, func(c *ColumnConfig) bool { return c.ID == id })
	if colIndex != -1 {
		col := tableConfig.Columns[colIndex]
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
