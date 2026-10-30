package display

import (
	_ "embed"

	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	"gopkg.in/yaml.v3"
)

type ColumnConfig struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`

	Group      string   `yaml:"group,omitempty" json:"group,omitempty"`
	Field      string   `yaml:"field,omitempty" json:"field,omitempty"`
	Fields     []string `yaml:"fields,omitempty" json:"fields,omitempty"`
	Calculated string   `yaml:"calculated,omitempty" json:"calculated,omitempty"`
	Tooltip    string   `yaml:"tooltip,omitempty" json:"tooltip,omitempty"`
	DocURL     string   `yaml:"docURL,omitempty" json:"docURL,omitempty"`
	Filter     string   `yaml:"filter,omitempty" json:"filter,omitempty"`
	Default    bool     `yaml:"default,omitempty" json:"default,omitempty"`
	Width      int      `yaml:"width,omitempty" json:"width,omitempty"`
	Feature    string   `yaml:"feature" json:"feature"`
}

type FilterConfig struct {
	ID        string `yaml:"id" json:"id"`
	Name      string `yaml:"name" json:"name"`
	Component string `yaml:"component" json:"component"`

	Category               string `yaml:"category,omitempty" json:"category,omitempty"`
	AutoCompleteAddsQuotes bool   `yaml:"autoCompleteAddsQuotes,omitempty" json:"autoCompleteAddsQuotes,omitempty"`
	Hint                   string `yaml:"hint,omitempty" json:"hint,omitempty"`
	Examples               string `yaml:"examples,omitempty" json:"examples,omitempty"`
	DocURL                 string `yaml:"docUrl,omitempty" json:"docUrl,omitempty"`
	Placeholder            string `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
}

type FieldConfig struct {
	Name        string `yaml:"name" json:"name"`
	Type        string `yaml:"type" json:"type"`
	Description string `yaml:"description" json:"description"`
	LokiLabel   bool   `yaml:"lokiLabel,omitempty" json:"lokiLabel,omitempty"`
	Filter      string `yaml:"filter,omitempty" json:"filter,omitempty"`
}

type Config struct {
	Columns []*ColumnConfig `yaml:"columns" json:"columns"`
	Fields  []*FieldConfig  `yaml:"fields" json:"fields"`
	Filters []*FilterConfig `yaml:"filters,omitempty" json:"filters,omitempty"`
}

var (
	//go:embed config.yaml
	rawConfig []byte
	cfg       Config
)

func LoadConfig() (Config, error) {
	if err := yaml.Unmarshal(rawConfig, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func CurrentConfig() Config { return cfg }

// ConfigureTable adapts the display schema to the table formatter used by
// flow and packet views.
func ConfigureTable(capture string, extraWidth int) {
	columns := make([]*collectorformat.ColumnConfig, len(cfg.Columns))
	for i, column := range cfg.Columns {
		columns[i] = &collectorformat.ColumnConfig{
			ID: column.ID, Name: column.Name, Group: column.Group,
			Field: column.Field, Width: column.Width,
		}
	}
	collectorformat.ConfigureTable(collectorformat.TableSettings{
		Columns: columns, RawDisplay: "Raw", ExtraWidth: extraWidth, Capture: capture,
	})
}
