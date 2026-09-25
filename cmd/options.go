package cmd

type option struct {
	all     []optionItem
	current int
}

type optionItem struct {
	name string
	ids  []string
}

var (
	allOptions = "All"
	noOptions  = "None"

	// displays
	rawDisplay              = "Raw"
	standardDisplay         = "Standard"
	pktDropFeature          = "pktDrop"
	dnsFeature              = "dnsTracking"
	rttFeature              = "flowRTT"
	networkEventsDisplay    = "networkEvents"
	pktTranslation          = "packetTranslation"
	udnMapping              = "udnMapping"
	ipSec                   = "ipsec"
	plaintextCaptureFeature = "plaintextCapture"

	defaultDisplayIndex = 1
	defaultPanelsIndex  = 0

	// columns displays
	display = option{
		all: []optionItem{
			// exclusive displays
			{name: rawDisplay},
			{name: standardDisplay},
			// per feature displays
			{name: "Packet drops", ids: []string{pktDropFeature}},
			{name: "DNS", ids: []string{dnsFeature}},
			{name: "RTT", ids: []string{rttFeature}},
			{name: "Network events", ids: []string{networkEventsDisplay}},
			{name: "Packet translation", ids: []string{pktTranslation}},
			{name: "UDN mapping", ids: []string{udnMapping}},
			{name: "IPSec", ids: []string{ipSec}},
			// all features display
			{name: allOptions, ids: []string{pktDropFeature, dnsFeature, rttFeature, networkEventsDisplay, pktTranslation, udnMapping, ipSec}},
		},
		// standard display by default
		current: defaultDisplayIndex,
	}

	// columns enrichments
	enrichment = option{
		all: []optionItem{
			// no enrichment
			{name: noOptions},
			// per field enrichments
			{name: "IP & Port", ids: []string{"SrcAddr", "SrcPort", "DstAddr", "DstPort"}},
			{name: "Cluster", ids: []string{"ClusterName"}},
			{name: "Zone", ids: []string{"SrcZone", "DstZone"}},
			{name: "Host", ids: []string{"SrcK8S_HostIP", "DstK8S_HostIP", "SrcK8S_HostName", "DstK8S_HostName", "FlowDirection"}},
			{name: "Namespace", ids: []string{"SrcK8S_Namespace", "DstK8S_Namespace"}},
			{name: "Owner", ids: []string{"SrcK8S_OwnerType", "DstK8S_OwnerType", "SrcK8S_OwnerName", "DstK8S_OwnerName", "SrcK8S_Namespace", "DstK8S_Namespace"}},
			{name: "Resource", ids: []string{"SrcK8S_Type", "DstK8S_Type", "SrcK8S_Name", "DstK8S_Name", "SrcK8S_Namespace", "DstK8S_Namespace"}},
			{name: "Subnet Label", ids: []string{"SrcSubnetLabel", "DstSubnetLabel"}},
			{name: "Network Name", ids: []string{"SrcNetworkName", "DstNetworkName"}},
			// all fields
			{name: allOptions, ids: []string{
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
		current: 7,
	}

	// panels
	panels = option{
		all: []optionItem{
			{name: "Nodes total", ids: []string{
				"sum(rate(on_demand_netobserv_node_egress_bytes_total[2m]))",
				"sum(rate(on_demand_netobserv_node_egress_packets_total[2m]))",
				"sum(rate(on_demand_netobserv_node_ingress_bytes_total[2m]))",
				"sum(rate(on_demand_netobserv_node_ingress_packets_total[2m]))",
			}},
			{name: "Infra namespaces total", ids: []string{
				"sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"infra\"}[2m]))",
			}},
			{name: "App namespaces total", ids: []string{
				"sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"app\"}[2m]))",
			}},
			{name: "Infra workloads total", ids: []string{
				"sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"infra\"}[2m]))",
			}},
			{name: "App workloads total", ids: []string{
				"sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"app\"}[2m]))",
			}},
			{name: "Top nodes", ids: []string{
				"topk(10,sum(rate(on_demand_netobserv_node_egress_bytes_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_egress_packets_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_ingress_bytes_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_ingress_packets_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
			}},
			{name: "Top namespaces (infra)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
			}},
			{name: "Top namespaces (app)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_egress_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_egress_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_ingress_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_ingress_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
			}},
			{name: "Top workloads (infra)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
			}},
			{name: "Top workloads (app)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_egress_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_egress_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_ingress_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_ingress_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
			}},
			{name: "Top nodes drop + total", ids: []string{
				"topk(10,sum(rate(on_demand_netobserv_node_drop_bytes_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_drop_packets_total[2m])) by (SrcK8S_HostName,DstK8S_HostName))",
				"sum(rate(on_demand_netobserv_node_drop_bytess_total[2m]))",
				"sum(rate(on_demand_netobserv_node_drop_packets_total[2m]))",
			}},
			{name: "Top namespaces drop + total (infra)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m]))",
			}},
			{name: "Top namespaces drop + total (app)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_namespace_drop_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_drop_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m]))",
			}},
			{name: "Top workloads drop + total (infra)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m]))",
			}},
			{name: "Top workloads drop + total (app)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
				"sum(rate(on_demand_netobserv_workload_drop_bytes_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_drop_packets_total{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m]))",
			}},
			{name: "DNS latency", ids: []string{
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_node_dns_latency_seconds_bucket[2m])) by (le)) > 0",
				"sum(rate(on_demand_netobserv_node_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\"}[2m]))",
			}},
			{name: "Top node DNS latencies", ids: []string{
				"topk(10,histogram_quantile(0.5, sum(rate(on_demand_netobserv_node_dns_latency_seconds_bucket[2m])) by (le,SrcK8S_HostName,DstK8S_HostName))*1000 > 0)",
				"topk(10,histogram_quantile(0.99, sum(rate(on_demand_netobserv_node_dns_latency_seconds_bucket[2m])) by (le,SrcK8S_HostName,DstK8S_HostName))*1000 > 0)",
				"topk(10,sum(rate(on_demand_netobserv_node_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_HostName,DstK8S_HostName))",
			}},
			{name: "Top namespaces DNS latencies (infra)", ids: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,DstK8S_Namespace)))",
			}},
			{name: "Top namespaces DNS latencies (app)", ids: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,DstK8S_Namespace)))",
			}},
			{name: "Top workloads DNS latencies (infra)", ids: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
			}},
			{name: "Top workloads DNS latencies (app)", ids: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_dns_latency_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)) or (sum(rate(on_demand_netobserv_workload_dns_latency_seconds_count{DnsFlagsResponseCode!=\"NoError\",K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (DnsFlagsResponseCode,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName)))",
			}},
			{name: "Top node RTT + total", ids: []string{
				"topk(10,histogram_quantile(0.5, sum(rate(on_demand_netobserv_node_rtt_seconds_bucket[2m])) by (le,SrcK8S_HostName,DstK8S_HostName))*1000 > 0)",
				"topk(10,histogram_quantile(0.99, sum(rate(on_demand_netobserv_node_rtt_seconds_bucket[2m])) by (le,SrcK8S_HostName,DstK8S_HostName))*1000 > 0)",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_node_rtt_seconds_bucket[2m])) by (le)) > 0",
			}},
			{name: "Top namespace RTT + total (infra)", ids: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le)) > 0",
			}},
			{name: "Top namespace RTT + total (app)", ids: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,DstK8S_Namespace))*1000 > 0))",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_namespace_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le)) > 0",
			}},
			{name: "Top workload RTT + total (infra)", ids: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (le)) > 0",
			}},
			{name: "Top workload RTT + total (app)", ids: []string{
				"topk(10,(histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.5, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"topk(10,(histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0) or (histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (le,SrcK8S_Namespace,SrcK8S_OwnerName,DstK8S_Namespace,DstK8S_OwnerName))*1000 > 0))",
				"histogram_quantile(0.99, sum(rate(on_demand_netobserv_workload_rtt_seconds_bucket{K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (le)) > 0",
			}},
			{name: "Top node network events + total", ids: []string{
				"topk(10,sum(rate(on_demand_netobserv_node_network_policy_events_total{action=~\"allow.*\"}[2m])) by (type,direction,SrcK8S_HostName,DstK8S_HostName))",
				"topk(10,sum(rate(on_demand_netobserv_node_network_policy_events_total{action=\"drop\"}[2m])) by (type,direction,SrcK8S_HostName,DstK8S_HostName))",
				"sum(rate(on_demand_netobserv_node_network_policy_events_total{action=~\"allow.*\"}[2m]))",
				"sum(rate(on_demand_netobserv_node_network_policy_events_total{action=\"drop\"}[2m]))",
			}},
			{name: "Top namespace network events + total (infra)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\",SrcK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\",DstK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\"}[2m]))",
			}},
			{name: "Top namespace network events + total (app)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)))",
				"topk(10,(sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\",SrcK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)) or (sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\",DstK8S_Namespace!=\"\"}[2m])) by (type,direction,SrcK8S_Namespace,DstK8S_Namespace)))",
				"sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_namespace_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\"}[2m]))",
			}},
			{name: "Top workload network events + total (infra)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\",SrcK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)) or (sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\",DstK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\",SrcK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)) or (sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\",DstK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)))",
				"sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"infra\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"infra\"}[2m]))",
			}},
			{name: "Top workload network events + total (app)", ids: []string{
				"topk(10,(sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\",SrcK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)) or (sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\",DstK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)))",
				"topk(10,(sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\",SrcK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)) or (sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\",DstK8S_workload!=\"\"}[2m])) by (type,direction,SrcK8S_workload,DstK8S_workload)))",
				"sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=~\"allow.*\",K8S_FlowLayer=\"app\"}[2m]))",
				"sum(rate(on_demand_netobserv_workload_network_policy_events_total{action=\"drop\",K8S_FlowLayer=\"app\"}[2m]))",
			}},
		},
		current: defaultPanelsIndex,
	}
)

func (opt *option) getCurrentItem() optionItem {
	return opt.all[opt.current]
}

func (opt *option) prev() {
	opt.current += len(opt.all) - 1
	opt.current %= len(opt.all)
}

func (opt *option) next() {
	opt.current++
	opt.current %= len(opt.all)
}
