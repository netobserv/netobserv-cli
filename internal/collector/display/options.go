package display

type Option struct {
	All     []OptionItem
	Current int
}

type OptionItem struct {
	Name string
	IDs  []string
}

var (
	allOptions = "All"
	noOptions  = "None"

	// displays
	rawDisplay           = "Raw"
	standardDisplay      = "Standard"
	pktDropFeature       = "pktDrop"
	dnsFeature           = "dnsTracking"
	rttFeature           = "flowRTT"
	networkEventsDisplay = "networkEvents"
	pktTranslation       = "packetTranslation"
	udnMapping           = "udnMapping"
	ipSec                = "ipsec"

	defaultDisplayIndex = 1
	defaultPanelsIndex  = 0

	// columns displays
	display = Option{
		All: []OptionItem{
			// exclusive displays
			{Name: rawDisplay},
			{Name: standardDisplay},
			// per feature displays
			{Name: "Packet drops", IDs: []string{pktDropFeature}},
			{Name: "DNS", IDs: []string{dnsFeature}},
			{Name: "RTT", IDs: []string{rttFeature}},
			{Name: "Network events", IDs: []string{networkEventsDisplay}},
			{Name: "Packet translation", IDs: []string{pktTranslation}},
			{Name: "UDN mapping", IDs: []string{udnMapping}},
			{Name: "IPSec", IDs: []string{ipSec}},
			// all features display
			{Name: allOptions, IDs: []string{pktDropFeature, dnsFeature, rttFeature, networkEventsDisplay, pktTranslation, udnMapping, ipSec}},
		},
		// standard display by default
		Current: defaultDisplayIndex,
	}

	// columns enrichments
	enrichment = Option{
		All: []OptionItem{
			// no enrichment
			{Name: noOptions},
			// per field enrichments
			{Name: "IP & Port", IDs: []string{"SrcAddr", "SrcPort", "DstAddr", "DstPort"}},
			{Name: "Cluster", IDs: []string{"ClusterName"}},
			{Name: "Zone", IDs: []string{"SrcZone", "DstZone"}},
			{Name: "Host", IDs: []string{"SrcK8S_HostIP", "DstK8S_HostIP", "SrcK8S_HostName", "DstK8S_HostName", "FlowDirection"}},
			{Name: "Namespace", IDs: []string{"SrcK8S_Namespace", "DstK8S_Namespace"}},
			{Name: "Owner", IDs: []string{"SrcK8S_OwnerType", "DstK8S_OwnerType", "SrcK8S_OwnerName", "DstK8S_OwnerName", "SrcK8S_Namespace", "DstK8S_Namespace"}},
			{Name: "Resource", IDs: []string{"SrcK8S_Type", "DstK8S_Type", "SrcK8S_Name", "DstK8S_Name", "SrcK8S_Namespace", "DstK8S_Namespace"}},
			{Name: "Subnet Label", IDs: []string{"SrcSubnetLabel", "DstSubnetLabel"}},
			{Name: "Network Name", IDs: []string{"SrcNetworkName", "DstNetworkName"}},
			// all fields
			{Name: allOptions, IDs: []string{
				"ClusterName",
				"SrcZone", "DstZone",
				"SrcK8S_HostIP", "DstK8S_HostIP", "SrcK8S_HostName", "DstK8S_HostName",
				"SrcK8S_Namespace", "DstK8S_Namespace",
				"SrcK8S_OwnerType", "DstK8S_OwnerType", "SrcK8S_OwnerName", "DstK8S_OwnerName",
				"SrcK8S_Type", "DstK8S_Type", "SrcK8S_Name", "DstK8S_Name",
				"SrcSubnetLabel", "DstSubnetLabel",
				"SrcNetworkName", "DstNetworkName",
			}},
		},
		// resource enrichment by default
		Current: 7,
	}

	// panels
	panels = Option{
		All: []OptionItem{
			{Name: "Nodes total", IDs: []string{
				"sum(rate(on_demand_netobserv_node_egress_bytes_total[2m]))",
				"sum(rate(on_demand_netobserv_node_egress_packets_total[2m]))",
				"sum(rate(on_demand_netobserv_node_ingress_bytes_total[2m]))",
				"sum(rate(on_demand_netobserv_node_ingress_packets_total[2m]))",
			}},
			{Name: "Infra namespaces total", IDs: []string{
				"sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"infra\"}[2m]))",
			}},
			{Name: "App namespaces total", IDs: []string{
				"sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"app\"}[2m]))",
			}},
			{Name: "Infra workloads total", IDs: []string{
				"sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"infra\"}[2m]))",
			}},
			{Name: "App workloads total", IDs: []string{
				"sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"app\"}[2m]))",
			}},
			{Name: "Top nodes", IDs: []string{
				"topk(10,sum(rate(on_demand_netobserv_node_egress_bytes_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_egress_packets_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_ingress_bytes_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_ingress_packets_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
			}},
			{Name: "Top namespaces (infra)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
			}},
			{Name: "Top namespaces (app)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
			}},
			{Name: "Top workloads (infra)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
			}},
			{Name: "Top workloads (app)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
			}},
			{Name: "Top nodes drop + total", IDs: []string{
				"topk(10,sum(rate(on_demand_netobserv_node_drop_bytes_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_drop_packets_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"sum(rate(on_demand_netobserv_node_drop_bytess_total[2m]))",
				"sum(rate(on_demand_netobserv_node_drop_packets_total[2m]))",
			}},
			{Name: "Top namespaces drop + total (infra)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m]))",
			}},
			{Name: "Top namespaces drop + total (app)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m]))",
			}},
			{Name: "Top workloads drop + total (infra)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m]))",
			}},
			{Name: "Top workloads drop + total (app)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m]))",
			}},
			{Name: "DNS latency", IDs: []string{
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_node_dns_latency_seconds_bucket[2m])) by (le)) > 0",
				"sum(rate(on_demand_netobserv_node_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\"}[2m]))",
			}},
			{Name: "Top node DNS latencies", IDs: []string{
				"topk(10,histogram_quantile(0.5, sum(rate(on_demand_netobserv_node_dns_latency_seconds_bucket[2m])) by (le,SrcK8S_HostName,DstK8S_HostName))*1000 > 0)",
				"topk(10,histogram_quantile(0.99, sum(rate(on_demand_netobserv_node_dns_latency_seconds_bucket[2m])) by (le,SrcK8S_HostName,DstK8S_HostName))*1000 > 0)",
				"topk(10,sum(rate(on_demand_netobserv_node_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_HostName,DstK8S_HostName))",
			}},
			{Name: "Top namespaces DNS latencies (infra)", IDs: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,DstK8S_Namespace)))",
			}},
			{Name: "Top namespaces DNS latencies (app)", IDs: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,DstK8S_Namespace)))",
			}},
			{Name: "Top workloads DNS latencies (infra)", IDs: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
			}},
			{Name: "Top workloads DNS latencies (app)", IDs: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
			}},
			{Name: "Top node RTT + total", IDs: []string{
				"topk(10,histogram_quantile(0.5, sum(rate(on_demand_netobserv_node_rtt_seconds_bucket[2m])) by (le,SrcK8S_HostName,DstK8S_HostName))*1000 > 0)",
				"topk(10,histogram_quantile(0.99, sum(rate(on_demand_netobserv_node_rtt_seconds_bucket[2m])) by (le,SrcK8S_HostName,DstK8S_HostName))*1000 > 0)",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_node_rtt_seconds_bucket[2m])) by (le)) > 0",
			}},
			{Name: "Top namespace RTT + total (infra)", IDs: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le)) > 0",
			}},
			{Name: "Top namespace RTT + total (app)", IDs: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le)) > 0",
			}},
			{Name: "Top workload RTT + total (infra)", IDs: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le)) > 0",
			}},
			{Name: "Top workload RTT + total (app)", IDs: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le)) > 0",
			}},
			{Name: "Top node network events + total", IDs: []string{
				"topk(10,sum(rate(on_demand_netobserv_node_network_policy_events_total{action=~\"allow.*\"}[2m])) by (type,direction,SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_network_policy_events_total{action=\"drop\"}[2m])) by (type,direction,SrcK8S_HostName,DstK8S_HostName))",
				"sum(rate(on_demand_netobserv_node_network_policy_events_total{action=~\"allow.*\"}[2m]))",
				"sum(rate(on_demand_netobserv_node_network_policy_events_total{action=\"drop\"}[2m]))",
			}},
			{Name: "Top namespace network events + total (infra)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\"}[2m]))",
			}},
			{Name: "Top namespace network events + total (app)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\"}[2m]))",
			}},
			{Name: "Top workload network events + total (infra)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\",SrcK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)) or (sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\",DstK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\",SrcK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)) or (sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\",DstK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)))",
				"sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\"}[2m]))",
			}},
			{Name: "Top workload network events + total (app)", IDs: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\",SrcK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)) or (sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\",DstK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\",SrcK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)) or (sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\",DstK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)))",
				"sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\"}[2m]))",
			}},
		},
		Current: defaultPanelsIndex,
	}
)

func (opt *Option) CurrentItem() OptionItem {
	return opt.All[opt.Current]
}

func (opt *Option) Prev() {
	opt.Current += len(opt.All) - 1
	opt.Current %= len(opt.All)
}

func (opt *Option) Next() {
	opt.Current++
	opt.Current %= len(opt.All)
}

func Defaults() (Option, Option, Option) {
	return display, enrichment, panels
}
