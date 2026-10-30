package packet

import (
	"net"
	"strings"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

// Filters holds CLI flow-filter hints used for plaintext ↔ wire correlation.
type Filters struct {
	Ports    []uint16
	PeerIPs  []net.IP
	PeerNets []*net.IPNet
}

func (f Filters) active() bool {
	return len(f.Ports) > 0 || len(f.PeerIPs) > 0 || len(f.PeerNets) > 0
}

// ParseFilters parses the pipe-separated capture options used by the CLI.
func ParseFilters(options string) Filters {
	var f Filters
	for _, key := range []string{"port", "dport", "sport"} {
		if v := optionValue(options, key); v != "" {
			if p, ok := parsePortString(v); ok {
				f.Ports = appendUniquePort(f.Ports, p)
			}
		}
	}
	if v := optionValue(options, "peer_ip"); v != "" {
		if ip := net.ParseIP(v); ip != nil {
			f.PeerIPs = append(f.PeerIPs, ip)
		}
	}
	if v := optionValue(options, "peer_cidr"); v != "" {
		_, n, err := net.ParseCIDR(v)
		if err == nil {
			f.PeerNets = append(f.PeerNets, n)
		}
	}
	return f
}

func appendUniquePort(ports []uint16, p uint16) []uint16 {
	for _, existing := range ports {
		if existing == p {
			return ports
		}
	}
	return append(ports, p)
}

// optionValue reads a named CLI option from the pipe-separated --options string.
func optionValue(options, name string) string {
	for _, part := range strings.Split(options, "|") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "--")
		if strings.HasPrefix(part, name+"=") {
			return strings.TrimPrefix(part, name+"=")
		}
	}
	return ""
}

func enrichPlaintextFromCaptureFilters(m *config.GenericMap, f Filters) {
	if m == nil || plaintextHasTuple(*m) {
		return
	}
	peerIP := singleCapturePeerIP(f)
	port := singleCapturePort(f)
	if peerIP != nil {
		applyWorkloadPlaintextTuple(m, peerIP, port)
		return
	}
	if port > 0 {
		// The agent keeps the local endpoint first, including read events.
		// A capture port must not replace an already known local port.
		if p, ok := mapPortFromGeneric(*m, "SrcPort"); ok && p > 0 {
			return
		}
		(*m)["Proto"] = float64(6)
		(*m)["SrcPort"] = port
	}
}

func singleCapturePeerIP(f Filters) net.IP {
	if len(f.PeerIPs) == 1 {
		return f.PeerIPs[0]
	}
	return nil
}

func singleCapturePort(f Filters) uint16 {
	if len(f.Ports) == 1 {
		return f.Ports[0]
	}
	return 0
}

func applyWorkloadPlaintextTuple(m *config.GenericMap, peer net.IP, port uint16) {
	if m == nil || peer == nil {
		return
	}
	if src, ok := validIPString((*m)["SrcAddr"]); ok && src != peer.String() {
		return
	}
	(*m)["Proto"] = float64(6)
	(*m)["SrcAddr"] = peer.String()
	if p, ok := mapPortFromGeneric(*m, "SrcPort"); port > 0 && (!ok || p == 0) {
		(*m)["SrcPort"] = port
	}
}

func wirePacketMatchesCaptureFilters(m config.GenericMap, f Filters) bool {
	if !f.active() {
		return false
	}
	if len(f.Ports) > 0 {
		matched := false
		for _, p := range f.Ports {
			if sp, ok := mapPortFromGeneric(m, "SrcPort"); ok && sp == p {
				matched = true
				break
			}
			if dp, ok := mapPortFromGeneric(m, "DstPort"); ok && dp == p {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(f.PeerIPs) > 0 {
		matched := false
		for _, ip := range f.PeerIPs {
			if wirePacketHasIP(m, ip.String()) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(f.PeerNets) > 0 {
		matched := false
		src, _ := validIPString(m["SrcAddr"])
		dst, _ := validIPString(m["DstAddr"])
		for _, n := range f.PeerNets {
			if ipInCaptureNet(src, n) || ipInCaptureNet(dst, n) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func ipInCaptureNet(ip string, n *net.IPNet) bool {
	if ip == "" || n == nil {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return n.Contains(parsed)
}
