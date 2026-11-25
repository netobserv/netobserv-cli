package agent

import (
	"fmt"
	"sort"

	flpapi "github.com/netobserv/flowlogs-pipeline/pkg/api"
)

// EnableUDNMapping replaces the --enable_udn_mapping branch of parse_args: marks the DaemonSet
// privileged and points the FLP "enrich" stage's Kubernetes secondary-network indexing at
// ovn-kubernetes's user-defined networks.
func (b *Builder) EnableUDNMapping() error {
	b.EnablePrivileged()

	enrich, err := b.pipelineStage("enrich")
	if err != nil {
		return err
	}
	if enrich.Transform == nil || enrich.Transform.Network == nil {
		return fmt.Errorf("FLP 'enrich' stage has no network transform configuration")
	}
	enrich.Transform.Network.KubeConfig.SecondaryNetworks = []flpapi.SecondaryNetwork{
		{Name: "ovn-kubernetes", Index: map[string]any{"udn": nil}},
	}

	b.setEnv(envUDNMapping, "true")
	return nil
}

// SetSubnetLabels replaces the --get-subnets branch of parse_args' "get_subnets" case: adds the
// SrcAddr/DstAddr subnet-labeling rules and the discovered CIDRs (see internal/plugin/subnets) to
// the FLP "enrich" stage. subnetsByCategory's keys become label names ("Machines", "Pods",
// "Services"); iterated in sorted order for deterministic output (bash's associative-array
// iteration order was unspecified).
func (b *Builder) SetSubnetLabels(subnetsByCategory map[string][]string) error {
	if len(subnetsByCategory) == 0 {
		return nil
	}

	enrich, err := b.pipelineStage("enrich")
	if err != nil {
		return err
	}
	if enrich.Transform == nil || enrich.Transform.Network == nil {
		return fmt.Errorf("FLP 'enrich' stage has no network transform configuration")
	}

	enrich.Transform.Network.Rules = append(enrich.Transform.Network.Rules,
		flpapi.NetworkTransformRule{
			Type:           flpapi.NetworkAddSubnetLabel,
			AddSubnetLabel: &flpapi.NetworkAddSubnetLabelRule{Input: "SrcAddr", Output: "SrcSubnetLabel"},
		},
		flpapi.NetworkTransformRule{
			Type:           flpapi.NetworkAddSubnetLabel,
			AddSubnetLabel: &flpapi.NetworkAddSubnetLabelRule{Input: "DstAddr", Output: "DstSubnetLabel"},
		},
	)

	categories := make([]string, 0, len(subnetsByCategory))
	for category := range subnetsByCategory {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	labels := make([]flpapi.NetworkTransformSubnetLabel, 0, len(categories))
	for _, category := range categories {
		labels = append(labels, flpapi.NetworkTransformSubnetLabel{
			Name:  category,
			CIDRs: subnetsByCategory[category],
		})
	}
	enrich.Transform.Network.SubnetLabels = labels

	return nil
}
