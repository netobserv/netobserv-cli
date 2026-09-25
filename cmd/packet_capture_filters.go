package cmd

import (
	"net"
	"strings"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

// captureFilters holds CLI flow-filter hints used for plaintext ↔ wire correlation.
type captureFilters struct {
	ports    []uint16
	peerIPs  []net.IP
	peerNets []*net.IPNet
}

func (f captureFilters) active() bool {
	return len(f.ports) > 0 || len(f.peerIPs) > 0 || len(f.peerNets) > 0
}

func parseCaptureFilters() captureFilters {
	var f captureFilters
	for _, key := range []string{"port", "dport", "sport"} {
		if v := optionValue(key); v != "" {
			if p, ok := parsePortString(v); ok {
				f.ports = appendUniquePort(f.ports, p)
			}
		}
	}
	if v := optionValue("peer_ip"); v != "" {
		if ip := net.ParseIP(v); ip != nil {
			f.peerIPs = append(f.peerIPs, ip)
		}
	}
	if v := optionValue("peer_cidr"); v != "" {
		_, n, err := net.ParseCIDR(v)
		if err == nil {
			f.peerNets = append(f.peerNets, n)
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
func optionValue(name string) string {
	for _, part := range strings.Split(options, "|") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "--")
		if strings.HasPrefix(part, name+"=") {
			return strings.TrimPrefix(part, name+"=")
		}
	}
	return ""
}

func enrichPlaintextFromCaptureFilters(m *config.GenericMap, f captureFilters) {
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
		dir, _ := (*m)["Direction"].(string)
		(*m)["Proto"] = float64(6)
		if dir == "read" {
			(*m)["DstPort"] = port
		} else {
			(*m)["SrcPort"] = port
		}
	}
}

func singleCapturePeerIP(f captureFilters) net.IP {
	if len(f.peerIPs) == 1 {
		return f.peerIPs[0]
	}
	return nil
}

func singleCapturePort(f captureFilters) uint16 {
	if len(f.ports) == 1 {
		return f.ports[0]
	}
	return 0
}

func applyWorkloadPlaintextTuple(m *config.GenericMap, peer net.IP, port uint16) {
	if m == nil || peer == nil {
		return
	}
	(*m)["Proto"] = float64(6)
	(*m)["SrcAddr"] = peer.String()
	if port > 0 {
		(*m)["SrcPort"] = port
	}
}

func wirePacketMatchesCaptureFilters(m config.GenericMap, f captureFilters) bool {
	if !f.active() {
		return false
	}
	if len(f.ports) > 0 {
		matched := false
		for _, p := range f.ports {
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
	if len(f.peerIPs) > 0 {
		matched := false
		for _, ip := range f.peerIPs {
			if wirePacketHasIP(m, ip.String()) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(f.peerNets) > 0 {
		matched := false
		src, _ := validIPString(m["SrcAddr"])
		dst, _ := validIPString(m["DstAddr"])
		for _, n := range f.peerNets {
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
