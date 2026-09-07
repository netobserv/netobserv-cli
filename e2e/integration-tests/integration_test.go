//go:build !e2e

package integrationtests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/netobserv/network-observability-cli/e2e"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sirupsen/logrus"
)

var (
	cliNS = "netobserv-cli"

	clientset  *kubernetes.Clientset
	filePrefix string
	ilog       = logrus.WithField("component", "integration_test")
	re         *regexp.Regexp
)

func writeOutput(filename string, out string) {
	ilog.Debugf("Writing %s...", filename)

	err := os.WriteFile(path.Join(os.Getenv("ARTIFACT_DIR"), filename), []byte(out), 0666)
	ilog.Info(fmt.Sprintf("Wrote file to path %s", path.Join(os.Getenv("ARTIFACT_DIR"), filename)))
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error writing command output %v", err))
}

func cleanup() {
	ilog.Info("Cleaning up...")

	// stop any capture still running from StartCommand, so it can't run its own EXIT trap
	// while the next spec is setting up
	e2e.StopStartedCommands(ilog)

	// rename dir flow with filename prefix
	itlog.Debugf("Removing %s", outputDir)
	err := os.RemoveAll(outputDir)
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Couldn't remove %s: %v", outputDir, err))

	// run cli to cleanup namespace
	cliArgs := []string{"cleanup"}
	out, err := e2e.RunCommand(ilog, ocNetObservBinPath, cliArgs...)
	writeOutput(filePrefix+"-cleanupOutput", out)
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error during cleanup %v", err))

	// ensure namespace is fully removed before next test to avoid error
	_, err = isNamespace(clientset, cliNS, false)
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Namespace wasn't deleted %v", err))

	ilog.Debug("Cleaned up !")
}

// keep this spec ordered to name cleanup files according to command run ones
var _ = g.Describe("NetObserv CLI e2e integration test suite", g.Ordered, func() {

	g.BeforeAll(func() {
		c, err := getNewClient()
		o.Expect(err).NotTo(o.HaveOccurred())
		clientset = c
	})
	g.BeforeEach(func(ctx g.SpecContext) {
		re = regexp.MustCompile(`OCP-\d+`)
		var filePrefixestring []string
		filePrefixestring = append(filePrefixestring, re.FindString(ctx.SpecReport().FullText()))
		if ctx.SpecReport().Labels() != nil {
			filePrefixestring = append(filePrefixestring, ctx.SpecReport().Labels()[0])
		}
		filePrefix = strings.Join(filePrefixestring, "-")
	})

	g.It("OCP-73458: Verify all CLI pods are deployed", g.Label("Sanity"), func() {
		g.DeferCleanup(func() {
			cleanup()
		})

		cliArgs := []string{"flows", "--copy=false", "--max-time=1m"}
		out, err := e2e.StartCommand(ilog, ocNetObservBinPath, cliArgs...)
		writeOutput(filePrefix+"-flowOutput", out)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error starting command %v", err))

		nodes, err := getClusterNodes(clientset, &metav1.ListOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = isCLIRuning(clientset, cliNS)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("CLI didn't come ready %v", err))
		allPods, err := getNamespacePods(context.Background(), clientset, cliNS, &metav1.ListOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		// agent pods + collector pods
		totalExpectedPods := len(nodes) + 1
		o.Expect(allPods).To(o.HaveLen(totalExpectedPods), fmt.Sprintf("Number of CLI pods are not as expected %d", totalExpectedPods))
	})

	g.It("OCP-73458: Verify packet capture creates pcapng file and filters by port", g.Label("PacketCapture"), func() {
		g.DeferCleanup(func() {
			cleanup()
		})

		// Run packet capture with port 8080 filter
		targetPort := uint16(8080)
		cliArgs := []string{"packets", "--port=8080", "--copy=true", "--max-bytes=100000000", "--max-time=1m"}
		out, err := e2e.RunCommand(ilog, ocNetObservBinPath, cliArgs...)
		writeOutput(filePrefix+"-packetOutput", out)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error running command %v", err))

		_, err = isCLIDone(clientset, cliNS)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("CLI didn't finish %v", err))

		// Verify pcapng file is created
		pcapngFile, err := getPcapngFile()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get pcapng file")
		o.Expect(pcapngFile).NotTo(o.BeEmpty(), "Pcapng file path should not be empty")

		ilog.Infof("==> Pcapng file created at: %s", pcapngFile)

		// Verify file exists and has content
		fileInfo, err := os.Stat(pcapngFile)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Pcapng file should exist at %s", pcapngFile))
		o.Expect(fileInfo.Size()).To(o.BeNumerically(">", 0), "Pcapng file should have content")

		ilog.Infof("==> Pcapng file size: %d bytes", fileInfo.Size())

		// Read and analyze packets from pcapng file
		packets, err := ReadPcapngFile(pcapngFile)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to read pcapng file: %v", err))
		o.Expect(packets).NotTo(o.BeEmpty(), "Pcapng file should contain packets")

		ilog.Infof("Found %d total packets in pcapng file", len(packets))
		// Verify packets are filtered by port 8080
		filteredPackets := FilterPacketsByPort(packets, targetPort)
		o.Expect(filteredPackets).NotTo(o.BeEmpty(), fmt.Sprintf("Should have captured packets on port %d", targetPort))

		ilog.Infof("Found %d packets on port %d", len(filteredPackets), targetPort)
	})
	g.It("OCP-73458: Verify regexes filters are applied", g.Label("Regexes"), func() {
		g.DeferCleanup(func() {
			cleanup()
		})

		// capture upto 500KB
		nsfilter := "openshift-monitoring"
		cliArgs := []string{"flows", fmt.Sprintf("--query=SrcK8S_Namespace=~\"%s\"", nsfilter), "--copy=true", "--max-bytes=500000", "--max-time=1m"}
		out, err := e2e.RunCommand(ilog, ocNetObservBinPath, cliArgs...)
		writeOutput(filePrefix+"-flowOutput", out)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error running command %v", err))

		_, err = isCLIDone(clientset, cliNS)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("CLI didn't finished %v", err))

		flowsJsonfile, err := getFlowsJSONFile()
		o.Expect(err).NotTo(o.HaveOccurred())
		flowsFile, err := os.Open(flowsJsonfile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer flowsFile.Close()

		decoder := json.NewDecoder(flowsFile)
		_, err = decoder.Token()
		o.Expect(err).NotTo(o.HaveOccurred())
		var flow struct {
			SrcK8sNamespace string `json:"SrcK8S_Namespace"`
		}

		found := false
		for decoder.More() {
			found = true
			err := decoder.Decode(&flow)
			o.Expect(err).NotTo(o.HaveOccurred())
			if flow.SrcK8sNamespace != nsfilter {
				o.Expect(true).To(o.BeFalse(), fmt.Sprintf("Flow %v does not meet regexes condition SrcK8S_Namespace=~%s", flow, nsfilter))
			}
		}
		o.Expect(found).To(o.BeTrue(), fmt.Sprintf("Didn't found any flow matching SrcK8S_Namespace=~%s", nsfilter))
	})
	g.It("OCP-82597: Verify sampling value of 1 is applied in captured flows", g.Label("Sampling"), func() {

		g.DeferCleanup(func() {
			cleanup()
		})

		// capture upto 500KB with sampling=1
		cliArgs := []string{"flows", "--sampling=1", "--copy=true", "--max-bytes=500000", "--max-time=1m"}
		out, err := e2e.RunCommand(ilog, ocNetObservBinPath, cliArgs...)
		writeOutput(filePrefix+"-flowOutput", out)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error running command %v", err))

		_, err = isCLIDone(clientset, cliNS)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("CLI didn't finished %v", err))

		flowsJsonfile, err := getFlowsJSONFile()
		o.Expect(err).NotTo(o.HaveOccurred())
		flowsFile, err := os.Open(flowsJsonfile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer flowsFile.Close()

		decoder := json.NewDecoder(flowsFile)
		_, err = decoder.Token()
		o.Expect(err).NotTo(o.HaveOccurred())
		var flow struct {
			Sampling float64 `json:"Sampling"`
		}

		atLeastOneFlowFound := false
		for decoder.More() {
			err := decoder.Decode(&flow)
			o.Expect(err).NotTo(o.HaveOccurred())
			atLeastOneFlowFound = true
			// Verify sampling value is 1
			o.Expect(flow.Sampling).To(o.BeNumerically("==", 1), fmt.Sprintf("Flow sampling value should be 1, but got %v", flow.Sampling))
		}
		o.Expect(atLeastOneFlowFound).To(o.BeTrue(), "At least one flow should be captured to verify sampling value")
	})

	g.It("OCP-82597: Verify excluded interface genev_sys_6081 does not appear in captured flows", g.Label("ExcludeInterface"), func() {
		g.DeferCleanup(func() {
			cleanup()
		})

		// capture upto 500KB with exclude_interfaces=genev_sys_6081
		cliArgs := []string{"flows", "--exclude_interfaces=genev_sys_6081", "--copy=true", "--max-bytes=500000", "--max-time=1m"}
		out, err := e2e.RunCommand(ilog, ocNetObservBinPath, cliArgs...)
		writeOutput(filePrefix+"-flowOutput", out)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error running command %v", err))

		_, err = isCLIDone(clientset, cliNS)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("CLI didn't finished %v", err))

		flowsJsonfile, err := getFlowsJSONFile()
		o.Expect(err).NotTo(o.HaveOccurred())
		flowsFile, err := os.Open(flowsJsonfile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer flowsFile.Close()

		decoder := json.NewDecoder(flowsFile)
		_, err = decoder.Token()
		o.Expect(err).NotTo(o.HaveOccurred())
		var flow struct {
			Interfaces []string `json:"Interfaces"`
		}

		for decoder.More() {
			err := decoder.Decode(&flow)
			o.Expect(err).NotTo(o.HaveOccurred())
			// Verify none of the flows contain genev_sys_6081
			for _, iface := range flow.Interfaces {
				o.Expect(iface).NotTo(o.Equal("genev_sys_6081"), fmt.Sprintf("Flow should not contain excluded interface genev_sys_6081, but found it in interfaces: %v", flow.Interfaces))
			}
		}
	})

	g.It("OCP-82598: Verify metrics command creates dashboard configmap and metrics are scraped", g.Label("Metrics"), func() {
		g.DeferCleanup(func() {
			cleanup()
		})

		// Run metrics command
		cliArgs := []string{"metrics", "--background"}
		out, err := e2e.StartCommand(ilog, ocNetObservBinPath, cliArgs...)
		writeOutput(filePrefix+"-flowOutput", out)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error starting command %v", err))

		// Wait for CLI to be ready
		daemonsetReady, err := isDaemonsetReady(clientset, "netobserv-cli", cliNS)
		o.Expect(err).NotTo(o.HaveOccurred(), "agent daemonset didn't come ready")
		o.Expect(daemonsetReady).To(o.BeTrue(), "agent daemonset didn't come ready")

		// Check that dashboard configmap is created
		dashboardCM, err := getConfigMap(context.Background(), clientset, "netobserv-cli", "openshift-config-managed")
		o.Expect(err).NotTo(o.HaveOccurred(), "Dashboard configmap should be created in openshift-config-managed namespace")
		o.Expect(dashboardCM.Name).To(o.Equal("netobserv-cli"), "Dashboard configmap should be named netobserv-cli")

		// Check that metrics are being scraped by Prometheus
		g.By("Verifying metrics are scraped by Prometheus")
		prometheusQuery := `sum(rate(on_demand_netobserv_node_egress_bytes_total[2m]))`
		metricValue, err := queryPrometheusMetric(clientset, prometheusQuery)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to query Prometheus for metrics: %v", err))
		o.Expect(metricValue).To(o.BeNumerically(">=", 0), fmt.Sprintf("Prometheus should return a valid metric value, but got %v", metricValue))
	})

	g.Describe("OCP-84801: Verify CLI runs under correct privileges", g.Label("Privileges"), func() {

		tests := []struct {
			when    string
			it      string
			cliArgs []string
			matcher types.GomegaMatcher
		}{
			{
				when:    "Executing `oc netobserv flows`",
				it:      "does not run as privileged",
				cliArgs: []string{"flows", "--background", "--max-time=1m"},
				matcher: o.BeFalse(),
			},
			{
				when:    "Executing `oc netobserv flows --privileged=true`",
				it:      "runs as privileged",
				cliArgs: []string{"flows", "--privileged=true", "--background", "--max-time=1m"},
				matcher: o.BeTrue(),
			},

			{
				when:    "Executing `oc netobserv flows --drops`",
				it:      "runs as privileged",
				cliArgs: []string{"flows", "--drops", "--background", "--max-time=1m"},
				matcher: o.BeTrue(),
			},
		}
		for i, t := range tests {
			g.When(t.when, func() {
				g.It(t.it, func() {
					filePrefix = filePrefix + "-" + strconv.Itoa(i)
					g.DeferCleanup(func() {
						cleanup()
					})
					// run command async until done
					out, err := e2e.StartCommand(ilog, ocNetObservBinPath, t.cliArgs...)
					writeOutput(filePrefix+"-flowOutput", out)
					o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error starting command %v", err))

					// Wait for CLI to be ready
					cliRunning, err := isCLIRuning(clientset, cliNS)
					o.Expect(cliRunning).To(o.BeTrue(), fmt.Sprintf("CLI didn't come ready %v", err))

					// Verify correct privilege setting
					ds, err := getDaemonSet(context.Background(), clientset, "netobserv-cli", cliNS)
					o.Expect(err).NotTo(o.HaveOccurred(), "DeamonSet should be created in CLI namespace")
					containers := ds.Spec.Template.Spec.Containers
					o.Expect(len(containers)).To(o.Equal(1), "The number of containers specified in the template is != 1")
					o.Expect(containers[0].SecurityContext.Privileged).To(o.HaveValue(t.matcher), "Priviledged is not set to true")
				})
			})

		}
	})
})
