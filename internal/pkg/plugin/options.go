package plugin

import (
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/netobserv/network-observability-cli/res"
	"k8s.io/apimachinery/pkg/util/validation"
)

type option struct{ key, value string }
type options struct {
	raw                                                          []string
	mode, namespace, kubeconfig, context, output, copy, logLevel string
	maxTime                                                      time.Duration
	maxBytes                                                     int64
	background, headless, yaml, subnets                          bool
	values                                                       []option
	filters                                                      []map[string]any
}

var filterFields = map[string]string{
	"direction": "direction", "cidr": "ip_cidr", "protocol": "protocol", "sport": "source_port", "dport": "destination_port", "port": "port",
	"sport_range": "source_port_range", "dport_range": "destination_port_range", "port_range": "port_range", "sports": "source_ports", "dports": "destination_ports", "ports": "ports",
	"icmp_type": "icmp_type", "icmp_code": "icmp_code", "peer_ip": "peer_ip", "peer_cidr": "peer_cidr", "action": "action", "tcp_flags": "tcp_flags", "drops": "drops",
}
var featureEnv = map[string]string{
	"enable_pkt_drop": "ENABLE_PKT_DROPS", "enable_dns": "ENABLE_DNS_TRACKING", "enable_rtt": "ENABLE_RTT", "enable_network_events": "ENABLE_NETWORK_EVENTS_MONITORING",
	"enable_udn_mapping": "ENABLE_UDN_MAPPING", "enable_pkt_translation": "ENABLE_PKT_TRANSLATION", "enable_ipsec": "ENABLE_IPSEC_TRACKING",
}
var enumerated = map[string][]string{
	"direction": {"Ingress", "Egress"}, "action": {"Accept", "Reject"}, "protocol": {"TCP", "UDP", "SCTP", "ICMP", "ICMPv6"},
	"tcp_flags": {"SYN", "SYN-ACK", "ACK", "FIN", "RST", "FIN-ACK", "RST-ACK", "PSH", "URG", "ECE", "CWR"},
	"log-level": {"trace", "debug", "info", "warn", "error", "fatal", "panic"}, "copy": {"true", "false", "prompt"},
}

func booleanOption(key string) bool {
	_, feature := featureEnv[key]
	return feature || oneOf(key, "background", "headless", "yaml", "get-subnets", "privileged", "enable_all", "drops", "enable_openssl")
}
func connectionOption(key string) bool {
	return oneOf(key, "namespace", "kubeconfig", "context", "output-dir")
}

func parseOptions(mode string, args []string) (*options, error) {
	o := &options{mode: mode, namespace: envDefault("NETOBSERV_NAMESPACE", "netobserv-cli"), output: "output", copy: envDefault("copy", "prompt"), logLevel: "info", maxTime: 5 * time.Minute, maxBytes: 50000000}
	o.background = envDefault("runBackground", "false") == "true"
	o.yaml = envDefault("outputYAML", "false") == "true"
	if mode == "metrics" {
		o.maxTime = time.Hour
	}
	b, err := res.Files.ReadFile("flow-filter.json")
	if err != nil {
		return nil, err
	}
	defaults := map[string]any{}
	if err = json.Unmarshal(b, &defaults); err != nil {
		return nil, err
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "or" {
			o.raw = append(o.raw, "or")
			o.filters = append(o.filters, maps.Clone(defaults))
			continue
		}
		opt, raw, next, err := readOption(args, i)
		if err != nil {
			return nil, err
		}
		i = next
		if err = validateOption(mode, opt); err != nil {
			return nil, err
		}
		if !connectionOption(opt.key) {
			o.raw = append(o.raw, raw)
		}
		if field, ok := filterFields[opt.key]; ok {
			o.applyFilter(opt, field, defaults)
		} else if err = o.applyOption(opt); err != nil {
			return nil, err
		}
		o.values = append(o.values, opt)
	}
	if len(validation.IsDNS1123Label(o.namespace)) > 0 {
		return nil, fmt.Errorf("invalid namespace %q", o.namespace)
	}
	if mode == "packets" && len(o.filters) == 0 {
		return nil, fmt.Errorf("at least one eBPF filter is required for packet capture")
	}
	return o, nil
}

func readOption(args []string, i int) (option, string, int, error) {
	raw := strings.TrimPrefix(args[i], "--")
	key, value, hasValue := strings.Cut(raw, "=")
	if !hasValue {
		if booleanOption(key) || key == "copy" {
			value = "true"
		} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			i++
			value = args[i]
			raw = key + "=" + value
		} else {
			return option{}, "", i, fmt.Errorf("missing value for --%s", key)
		}
	}
	return option{key, value}, raw, i, nil
}
func validateOption(mode string, opt option) error {
	key, value := opt.key, opt.value
	if booleanOption(key) && !oneOf(value, "true", "false") {
		return fmt.Errorf("--%s must be true or false", key)
	}
	if allowed, ok := enumerated[key]; ok && !oneOf(value, allowed...) {
		return fmt.Errorf("invalid --%s: %s", key, value)
	}
	if oneOf(key, "enable_openssl", "tls_plaintext_min_bytes", "tls_plaintext_preview_bytes", "tls_process_allowlist", "tls-keylog") && mode != "packets" {
		return fmt.Errorf("--%s is invalid for %s", key, mode)
	}
	_, feature := featureEnv[key]
	if mode == "packets" && (feature || oneOf(key, "sampling", "interfaces", "exclude_interfaces")) {
		return fmt.Errorf("--%s is invalid for packets", key)
	}
	if key == "include_list" && mode != "metrics" {
		return fmt.Errorf("include_list requires metrics")
	}
	if key == "max-bytes" && mode == "metrics" {
		return fmt.Errorf("--max-bytes is invalid for metrics")
	}
	return validateValue(opt)
}
func validateValue(opt option) error {
	key, value := opt.key, opt.value
	switch key {
	case "sport", "dport", "port", "icmp_type", "icmp_code", "sampling":
		limit := 65535
		if strings.HasPrefix(key, "icmp_") {
			limit = 255
		}
		if key == "sampling" {
			limit = 2147483647
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > limit {
			return fmt.Errorf("invalid --%s: %s", key, value)
		}
	case "cidr", "peer_cidr":
		if _, _, err := net.ParseCIDR(value); err != nil {
			return fmt.Errorf("invalid --%s: %w", key, err)
		}
	case "peer_ip":
		if net.ParseIP(value) == nil {
			return fmt.Errorf("invalid peer IP %q", value)
		}
	case "node-selector":
		k, v, ok := strings.Cut(value, ":")
		if !ok || len(validation.IsQualifiedName(k)) > 0 || len(validation.IsValidLabelValue(v)) > 0 {
			return fmt.Errorf("use --node-selector=key:value")
		}
	case "query":
		if value == "" {
			return fmt.Errorf("query must not be empty")
		}
	}
	return validatePlaintextValue(opt)
}
func (o *options) applyFilter(opt option, field string, defaults map[string]any) {
	if opt.key == "drops" && opt.value == "false" {
		return
	}
	if len(o.filters) == 0 {
		o.filters = append(o.filters, maps.Clone(defaults))
	}
	var value any = opt.value
	if oneOf(opt.key, "sport", "dport", "port", "icmp_type", "icmp_code") {
		value, _ = strconv.Atoi(opt.value)
	}
	if opt.key == "drops" {
		value = opt.value == "true"
	}
	o.filters[len(o.filters)-1][field] = value
}
func (o *options) applyOption(opt option) error {
	var err error
	switch opt.key {
	case "namespace":
		o.namespace = opt.value
	case "kubeconfig":
		o.kubeconfig = opt.value
	case "context":
		o.context = opt.value
	case "output-dir":
		o.output = opt.value
	case "headless":
		o.headless = opt.value == "true"
	case "background":
		o.background = opt.value == "true"
	case "yaml":
		o.yaml = opt.value == "true"
	case "copy":
		o.copy = opt.value
	case "get-subnets":
		o.subnets = o.subnets || opt.value == "true"
	case "max-time":
		o.maxTime, err = time.ParseDuration(opt.value)
	case "max-bytes":
		o.maxBytes, err = strconv.ParseInt(opt.value, 10, 64)
	case "log-level":
		o.logLevel = opt.value
	case "sampling", "node-selector", "query", "include_list", "interfaces", "exclude_interfaces", "privileged", "enable_all", "enable_openssl", "tls_plaintext_min_bytes", "tls_plaintext_preview_bytes", "tls_process_allowlist", "tls-keylog":
	default:
		if _, ok := featureEnv[opt.key]; !ok {
			return fmt.Errorf("unknown option --%s", opt.key)
		}
	}
	if err != nil {
		return fmt.Errorf("invalid --%s: %w", opt.key, err)
	}
	return nil
}
func oneOf(v string, choices ...string) bool {
	for _, c := range choices {
		if v == c {
			return true
		}
	}
	return false
}

// Bash feature flags are additive: false does not undo an earlier true,
// except enable_ipsec, which explicitly sets the environment value.
func (o *options) enabled(key string) bool {
	v := false
	_, feature := featureEnv[key]
	for _, opt := range o.values {
		if opt.key == key {
			if opt.value == "true" {
				v = true
			} else if key == "enable_ipsec" {
				v = false
			}
		}
		if opt.key == "enable_all" && opt.value == "true" && (feature || key == "privileged") {
			v = true
		}
	}
	return v
}
