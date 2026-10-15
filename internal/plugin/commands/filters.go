package commands

import (
	"fmt"
	"strconv"
	"strings"

	agentconfig "github.com/netobserv/netobserv-ebpf-agent/pkg/config"

	"github.com/netobserv/network-observability-cli/internal/plugin/manifests/agent"
)

// Bash's flow-filter DSL (scripts/functions.sh's parse_args/edit_manifest "filter_*" cases) is a
// small stateful grammar: a sequence of --flag=value tokens builds up "the current filter", and
// a bare `or` token starts a new one. That doesn't map onto Cobra/pflag's flag registration
// (pflag has no notion of "groups separated by a bare positional token", and a repeated
// non-slice flag is simply last-value-wins). Rather than force this into pflag, this file
// implements the same small sequential grammar directly and hands everything else to Cobra's
// normal flag parser untouched.
//
// Also replicates a real constraint of the original syntax: scripts/functions.sh's parser only
// ever supported "--key=value" (bash's `${option%%=*}`/`${option#*=}`), never space-separated
// "--key value" — so this parser doesn't need to either.

// filterFlagNames are the flag keys that belong to a flow-filter group rather than being a
// normal global flag, mirroring parse_args' "filter_*" cases. "drops" is included here (it also
// sets the eBPF flow filter's Drops field) even though, like the boolean enable_* flags, bare
// `--drops` (no value) means true.
var filterFlagNames = map[string]bool{
	"direction": true, "cidr": true, "protocol": true,
	"sport": true, "dport": true, "port": true,
	"sport_range": true, "dport_range": true, "port_range": true,
	"sports": true, "dports": true, "ports": true,
	"tcp_flags": true, "drops": true,
	"icmp_type": true, "icmp_code": true,
	"peer_ip": true, "peer_cidr": true, "action": true,
}

var validDirections = map[string]bool{"Ingress": true, "Egress": true}
var validProtocols = map[string]bool{"TCP": true, "UDP": true, "SCTP": true, "ICMP": true, "ICMPv6": true}

var validTCPFlags = map[string]bool{
	"SYN": true, "SYN-ACK": true, "ACK": true, "FIN": true, "RST": true,
	"FIN-ACK": true, "RST-ACK": true, "PSH": true, "URG": true, "ECE": true, "CWR": true,
}

var validActions = map[string]bool{"Accept": true, "Reject": true}

// filterToken is one --key=value (or, for "drops" only, bare --key) occurrence.
type filterToken struct {
	key   string
	value string
}

// splitFilterArgs partitions raw command-line args into:
//   - groups: filter-group tokens, split at each bare "or", in original order (always at least
//     one, possibly empty, group)
//   - queries: every --query=value occurrence in original order (query mutates the FLP pipeline
//     directly, independent of the eBPF flow-filter array, so it is never grouped)
//   - rest: everything else, unchanged and in original order, to hand to Cobra's normal parser
func splitFilterArgs(args []string) (groups [][]filterToken, queries []string, rest []string) {
	groups = [][]filterToken{{}}
	for _, arg := range args {
		if arg == "or" {
			groups = append(groups, []filterToken{})
			continue
		}
		if !strings.HasPrefix(arg, "--") {
			rest = append(rest, arg)
			continue
		}

		key, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		switch {
		case key == "query":
			queries = append(queries, value)
		case key == "drops" && !hasValue:
			groups[len(groups)-1] = append(groups[len(groups)-1], filterToken{key: key, value: "true"})
		case filterFlagNames[key]:
			groups[len(groups)-1] = append(groups[len(groups)-1], filterToken{key: key, value: value})
		default:
			rest = append(rest, arg)
		}
	}
	return groups, queries, rest
}

// applyFilterGroups replaces parse_args' handling of filter_* options and the bare `or` token.
func applyFilterGroups(b *agent.Builder, groups [][]filterToken) error {
	filters := make([]agentconfig.FlowFilter, 0, len(groups))
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		filter := agentconfig.FlowFilter{IPCIDR: "0.0.0.0/0", Action: "Accept"}
		for _, tok := range group {
			if err := applyFilterToken(b, &filter, tok); err != nil {
				return err
			}
		}
		filters = append(filters, filter)
	}
	b.SetFlowFilters(filters)
	return nil
}

// applyFilterToken applies one CLI filter option directly to the typed agent
// filter. A switch is deliberately used here: the supported CLI grammar is
// small, and keeping each flag's destination visible is easier to maintain
// than several registries of function pointers.
func applyFilterToken(b *agent.Builder, filter *agentconfig.FlowFilter, tok filterToken) error {
	switch tok.key {
	case "cidr":
		filter.IPCIDR = tok.value
	case "sport_range":
		filter.SourcePortRange = tok.value
	case "dport_range":
		filter.DestinationPortRange = tok.value
	case "port_range":
		filter.PortRange = tok.value
	case "sports":
		filter.SourcePorts = tok.value
	case "dports":
		filter.DestinationPorts = tok.value
	case "ports":
		filter.Ports = tok.value
	case "peer_ip":
		filter.PeerIP = tok.value
	case "peer_cidr":
		filter.PeerCIDR = tok.value
	case "direction", "protocol", "tcp_flags", "action":
		return applyValidatedFilter(filter, tok)
	case "sport", "dport", "port":
		return applyPortFilter(filter, tok)
	case "icmp_type", "icmp_code":
		return applyICMPFilter(filter, tok)
	case "drops":
		return applyDropsFilter(b, filter, tok)
	default:
		return fmt.Errorf("unknown filter flag --%s", tok.key)
	}
	return nil
}

func applyValidatedFilter(filter *agentconfig.FlowFilter, tok filterToken) error {
	valid := map[string]map[string]bool{
		"direction": validDirections,
		"protocol":  validProtocols,
		"tcp_flags": validTCPFlags,
		"action":    validActions,
	}
	if !valid[tok.key][tok.value] {
		return invalidFilterValue(tok)
	}

	switch tok.key {
	case "direction":
		filter.Direction = tok.value
	case "protocol":
		filter.Protocol = tok.value
	case "tcp_flags":
		filter.TCPFlags = tok.value
	case "action":
		filter.Action = tok.value
	}
	return nil
}

func applyPortFilter(filter *agentconfig.FlowFilter, tok filterToken) error {
	n, err := strconv.ParseInt(tok.value, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid value for --%s: %q (must be a port number)", tok.key, tok.value)
	}

	switch tok.key {
	case "sport":
		filter.SourcePort = int32(n)
	case "dport":
		filter.DestinationPort = int32(n)
	case "port":
		filter.Port = int32(n)
	}
	return nil
}

func applyICMPFilter(filter *agentconfig.FlowFilter, tok filterToken) error {
	n, err := strconv.Atoi(tok.value)
	if err != nil {
		return fmt.Errorf("invalid value for --%s: %q (must be an integer)", tok.key, tok.value)
	}
	if tok.key == "icmp_type" {
		filter.ICMPType = n
	} else {
		filter.ICMPCode = n
	}
	return nil
}

func applyDropsFilter(b *agent.Builder, filter *agentconfig.FlowFilter, tok filterToken) error {
	drops, err := strconv.ParseBool(tok.value)
	if err != nil {
		return fmt.Errorf("invalid value for --drops: %q", tok.value)
	}
	if drops {
		b.EnablePrivileged()
		b.EnablePacketDrops()
	}
	filter.Drops = drops
	return nil
}

func invalidFilterValue(tok filterToken) error {
	return fmt.Errorf("invalid value for --%s: %q", tok.key, tok.value)
}
