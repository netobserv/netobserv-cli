package kindconfig

import (
	"strings"
	"testing"
)

func TestManualUsesEnvironmentOverrides(t *testing.T) {
	values := map[string]string{
		"IP_FAMILY":     "ipv4",
		"NET_CIDR_IPV4": "10.20.0.0/16",
		"NET_CIDR_IPV6": "fd00:20::/48",
		"SVC_CIDR_IPV4": "10.21.0.0/16",
		"SVC_CIDR_IPV6": "fd00:21::/112",
	}
	config := Manual(func(name string) string { return values[name] })

	if got, want := config.Networking.IPFamily, "ipv4"; got != want {
		t.Fatalf("IP family = %q, want %q", got, want)
	}
	if got, want := config.Networking.PodSubnet, "10.20.0.0/16,fd00:20::/48"; got != want {
		t.Fatalf("pod subnet = %q, want %q", got, want)
	}
	if got, want := config.Networking.ServiceSubnet, "10.21.0.0/16,fd00:21::/112"; got != want {
		t.Fatalf("service subnet = %q, want %q", got, want)
	}

	data, err := config.YAML()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"role: control-plane", "role: worker", "hostPath: /var/run/netns", "v: \"5\""} {
		if !strings.Contains(text, want) {
			t.Fatalf("manual config does not contain %q:\n%s", want, text)
		}
	}
}

func TestE2EConfigHasMinimalNode(t *testing.T) {
	config := E2E()
	if len(config.Nodes) != 1 || len(config.Nodes[0].ExtraPortMappings) != 0 {
		t.Fatalf("unexpected e2e node configuration: %#v", config.Nodes)
	}
}
