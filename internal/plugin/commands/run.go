package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
	sharedcapture "github.com/netobserv/netobserv-operator/pkg/capture"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
	"github.com/netobserv/network-observability-cli/internal/k8s/ocp"
	plugincapture "github.com/netobserv/network-observability-cli/internal/plugin/capture"
	"github.com/netobserv/network-observability-cli/internal/plugin/manifests"
	"github.com/netobserv/network-observability-cli/internal/plugin/manifests/agent"
	"github.com/netobserv/network-observability-cli/internal/plugin/subnets"
)

// versionRequirement replaces one of checkClusterVersion's per-feature `compare_versions`
// checks (e.g. "packet capture requires OpenShift 4.16 or higher").
type versionRequirement struct {
	feature    string
	minVersion string
}

// captureRun carries the state threaded through runCapture's steps, replacing
// commands/netobserv's setup()-through-exec main flow. Split into small methods (one per
// setup()/parse_args phase) purely to keep each one readable; runCapture below is still the
// single entry point and still executes every step in the same order bash did.
type captureRun struct {
	cmd        *cobra.Command
	kind       sharedcapture.Type
	global     *globalOptions
	capOpts    *captureOptions
	builder    *agent.Builder
	versionReq []versionRequirement
	rawArgs    []string

	ctx    context.Context
	out    io.Writer
	client *k8sclient.Client
	isOCP  bool
	tls    bool
	sink   k8sclient.Sink
}

// runCapture is the single entry point flows/packets/metrics call after building and mutating
// their agent.Builder from the user's flags.
func runCapture(cmd *cobra.Command, kind sharedcapture.Type, global *globalOptions, capOpts *captureOptions, builder *agent.Builder, versionReqs []versionRequirement, rawArgs []string) error {
	global.applyDefaults()
	r := &captureRun{
		cmd: cmd, kind: kind, global: global, capOpts: capOpts, builder: builder, versionReq: versionReqs, rawArgs: rawArgs,
		ctx: cmd.Context(), out: cmd.OutOrStdout(),
	}

	if err := r.validateCluster(); err != nil {
		return err
	}
	r.tls = r.isOCP && !capOpts.outputYAML && kind.HasCollector()
	if err := r.prepareBuilder(); err != nil {
		return err
	}

	namespaceObj, rbacBundle, ds, err := r.buildManifestObjects()
	if err != nil {
		return err
	}

	sink, closeSink, err := newSink(r.client, capOpts, kind, r.out)
	if err != nil {
		return err
	}
	defer closeSink()
	r.sink = sink

	if err := r.applyManifests(namespaceObj, rbacBundle, ds); err != nil {
		return err
	}
	if capOpts.outputYAML {
		printYAMLInstructions(r.out, kind)
		return nil
	}

	return r.runCollector(ds)
}

// validateCluster replaces setup()'s cluster-readiness block: connectivity, OpenShift/version
// detection, the pre-existing-namespace guard, and --node-selector validation. All skipped for
// --yaml, exactly as bash's `if outputYAML == false` guard did.
func (r *captureRun) validateCluster() error {
	if r.capOpts.outputYAML {
		return nil
	}

	client, err := k8sclient.New(r.global.kubeconfig, r.global.kubeContext)
	if err != nil {
		return err
	}
	r.client = client

	if err := client.IsReachable(); err != nil {
		return fmt.Errorf("you must be connected to a cluster: %w", err)
	}

	r.isOCP, err = ocp.Detect(client.Discovery)
	if err != nil {
		return err
	}
	if err := checkClusterVersion(r.ctx, client, r.isOCP, r.kind, r.versionReq, r.out); err != nil {
		return fmt.Errorf("remove incompatible features and try again: %w", err)
	}

	exists, err := client.NamespaceExists(r.ctx, r.global.namespace)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%s namespace already exists; ensure no one else is running a capture on this cluster, or run 'netobserv cleanup' first", r.global.namespace)
	}

	if r.capOpts.nodeSelector != "" {
		if err := checkNodeSelectorMatches(r.ctx, client, r.capOpts.nodeSelector, r.out); err != nil {
			return err
		}
	}
	return nil
}

// prepareBuilder replaces the handful of edit_manifest calls setup() makes itself (as opposed to
// parse_args' flag-driven ones, already applied by the caller): node selector, TLS, subnets.
func (r *captureRun) prepareBuilder() error {
	if r.capOpts.nodeSelector != "" {
		key, value := splitNodeSelector(r.capOpts.nodeSelector)
		r.builder.SetNodeSelector(key, value)
	}
	if r.tls {
		if err := r.builder.EnableCollectorTLS("/etc/collector-ca/service-ca.crt"); err != nil {
			return err
		}
	}
	if r.capOpts.getSubnets && !r.capOpts.outputYAML {
		discovered, err := subnets.Discover(r.ctx, r.client.Clientset, r.client.Dynamic)
		if err != nil {
			return fmt.Errorf("cannot discover cluster subnets: %w", err)
		}
		printSubnets(r.out, discovered)
		if err := r.builder.SetSubnetLabels(discovered); err != nil {
			return err
		}
	}
	return nil
}

func printSubnets(out io.Writer, discovered map[string][]string) {
	if len(discovered) == 0 {
		fmt.Fprintln(out, "Didn't find subnets")
		return
	}
	fmt.Fprintln(out, "Found subnets:")
	for category, cidrs := range discovered {
		fmt.Fprintf(out, "    %s: %v\n", category, cidrs)
	}
}

func (r *captureRun) buildManifestObjects() (namespaceObj *corev1.Namespace, rbacBundle []runtime.Object, ds *appsv1.DaemonSet, err error) {
	ns := manifests.BuildNamespace(r.global.namespace)
	if r.isOCP {
		if ns.Annotations == nil {
			ns.Annotations = map[string]string{}
		}
		// Override any defaultNodeSelector configured on the Scheduler CR (NETOBSERV-2845).
		ns.Annotations["openshift.io/node-selector"] = ""
	}

	// YAML output is cluster-independent and must remain applicable to OpenShift, where the
	// SecurityContextConstraints object is required. Live Kubernetes captures omit that object.
	rbac, err := manifests.BuildRBACBundle(r.global.namespace, r.isOCP || r.capOpts.outputYAML)
	if err != nil {
		return nil, nil, nil, err
	}

	builtDS, err := r.builder.Build()
	if err != nil {
		return nil, nil, nil, err
	}

	return ns, rbac, builtDS, nil
}

// applyManifests replaces setup()'s namespace/service-account/service/CA-configmap/
// service-monitor/daemonset apply sequence, deferring the DaemonSet until the collector is
// ready when TLS is enabled (see runCollector), exactly as setup() did.
func (r *captureRun) applyManifests(namespaceObj *corev1.Namespace, rbacBundle []runtime.Object, ds *appsv1.DaemonSet) error {
	fmt.Fprintf(r.out, "creating %s namespace\n", r.global.namespace)
	if err := r.sink.Apply(r.ctx, namespaceObj); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "creating service account")
	if err := r.sink.Apply(r.ctx, rbacBundle...); err != nil {
		return err
	}

	if r.kind.HasCollector() {
		if err := r.applyCollectorService(); err != nil {
			return err
		}
	}
	if r.kind == sharedcapture.Metrics {
		fmt.Fprintln(r.out, "creating service monitor")
		metricsBundle, err := manifests.BuildMetricsBundle(r.global.namespace)
		if err != nil {
			return err
		}
		if err := r.sink.Apply(r.ctx, metricsBundle...); err != nil {
			return err
		}
	}

	fmt.Fprintf(r.out, "creating %s-capture agents\n", singularLabel(r.kind))
	if !r.tls {
		if err := r.sink.Apply(r.ctx, ds); err != nil {
			return err
		}
	}
	return nil
}

func (r *captureRun) applyCollectorService() error {
	fmt.Fprintln(r.out, "creating collector service")
	svc := sharedcapture.BuildCollectorService("collector", r.global.namespace, map[string]string{"run": "collector"})
	if r.tls {
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		svc.Annotations["service.beta.openshift.io/serving-cert-secret-name"] = manifests.CACertSecretName
	}
	if err := r.sink.Apply(r.ctx, svc); err != nil {
		return err
	}
	if r.tls {
		fmt.Fprintln(r.out, "creating CA configmap for TLS")
		if err := r.sink.Apply(r.ctx, manifests.BuildCAConfigMap(r.global.namespace)); err != nil {
			return err
		}
	}
	return nil
}

// runCollector replaces the collector-pod-through-exec half of commands/netobserv: wait for the
// (non-TLS) agents, create and wait for the collector, deploy the (TLS) agents once the
// resolve-tls initContainer has run, then either exec into the collector or leave it running.
func (r *captureRun) runCollector(ds *appsv1.DaemonSet) error {
	if !r.tls {
		if err := plugincapture.WaitForDaemonSetReady(r.ctx, r.client, r.global.namespace, plugincapture.DaemonSetName, 10*time.Minute, r.out); err != nil {
			return err
		}
	}

	inPodArgs := buildInPodArgs(r.kind, r.capOpts, r.global.namespace, buildOptionString(r.rawArgs))
	podStartCommand := []string{"sleep", "infinity"}
	if r.capOpts.background {
		podStartCommand = []string{"bash", "-c", shellQuoteJoin(inPodArgs) + " && sleep infinity"}
	}

	fmt.Fprintln(r.out, "Running collector...")
	pod, err := plugincapture.CreateCollectorPod(r.ctx, r.client, &plugincapture.CollectorPodOptions{
		Namespace:      r.global.namespace,
		Image:          r.global.collectorImage,
		PullPolicy:     corev1.PullPolicy(r.global.pullPolicy),
		ServiceAccount: "netobserv-cli",
		Command:        podStartCommand,
		TLSEnabled:     r.tls,
	})
	if err != nil {
		return err
	}
	if err := plugincapture.WaitForPodReady(r.ctx, r.client, r.global.namespace, pod.Name, 2*time.Minute); err != nil {
		return err
	}

	if r.tls {
		fmt.Fprintln(r.out, "creating capture agents")
		if err := r.sink.Apply(r.ctx, ds); err != nil {
			return err
		}
		if err := plugincapture.WaitForDaemonSetReady(r.ctx, r.client, r.global.namespace, plugincapture.DaemonSetName, 10*time.Minute, r.out); err != nil {
			return err
		}
	}

	if r.capOpts.background {
		printBackgroundInstructions(r.ctx, r.out, r.kind, r.capOpts, r.client)
		return nil
	}

	fmt.Fprintln(r.out, "Executing collector command...")
	namespace := r.global.namespace
	client := r.client
	runCtx := r.ctx
	out := r.out
	defer func() {
		exitOpts := *r.capOpts
		if runCtx.Err() != nil {
			// A signal already interrupted this capture (see cmd/plugin/main.go's
			// signal.NotifyContext): don't also block tearing it down on a second interactive
			// prompt nothing will ever answer, matching how Ctrl+C is expected to behave in
			// most CLIs (clean up and exit, not ask another question first).
			exitOpts.copyMode = "false"
		}
		_ = onCaptureExit(context.Background(), client, namespace, r.kind, &exitOpts, out)
	}()
	return plugincapture.ExecInPod(r.ctx, r.client, &plugincapture.ExecOptions{
		Namespace: r.global.namespace,
		Pod:       pod.Name,
		Command:   inPodArgs,
		Stdin:     r.cmd.InOrStdin(),
		Stdout:    r.out,
		Stderr:    r.cmd.ErrOrStderr(),
		TTY:       true,
	})
}

// newSink replaces applyYAML's branch on outputYAML: either every manifest is applied straight
// to the cluster, or rendered to ./output/<kind>_capture_<date>.yml instead.
func newSink(client *k8sclient.Client, capOpts *captureOptions, kind sharedcapture.Type, out io.Writer) (k8sclient.Sink, func(), error) {
	if !capOpts.outputYAML {
		return &k8sclient.ClusterSink{Client: client}, func() {}, nil
	}

	if err := os.MkdirAll("output", 0o755); err != nil {
		return nil, nil, fmt.Errorf("cannot create output directory: %w", err)
	}
	dateName := time.Now().Format("2006_01_02_03_04")
	path := filepath.Join("output", fmt.Sprintf("%s_capture_%s.yml", kind, dateName))
	f, err := os.Create(path) //nolint:gosec // path is built from a fixed prefix and a timestamp, not user input
	if err != nil {
		return nil, nil, fmt.Errorf("cannot create %s: %w", path, err)
	}
	fmt.Fprintln(out, "Output YAMLs without applying...")
	return k8sclient.NewYAMLWriter(f), func() { _ = f.Close() }, nil
}

func checkClusterVersion(ctx context.Context, client *k8sclient.Client, isOCP bool, kind sharedcapture.Type, reqs []versionRequirement, out io.Writer) error {
	if !isOCP {
		fmt.Fprintln(out, "Skipping OpenShift version check: cluster is not OpenShift")
		return nil
	}
	cv, err := ocp.GetClusterVersion(ctx, client.Dynamic)
	if err != nil {
		return err
	}
	version, ok := ocp.CompletedVersion(cv)
	if !ok {
		fmt.Fprintln(out, "Warning: can't find current version in the clusterversion history. Is the cluster upgrading?")
		return nil
	}
	fmt.Fprintf(out, "OpenShift version: %s\n", version)

	current, err := semver.NewVersion(version)
	if err != nil {
		return nil //nolint:nilerr // an unparsable version string shouldn't block the capture; just skip gating
	}

	allReqs := append([]versionRequirement{}, reqs...)
	if kind == sharedcapture.Packets {
		allReqs = append(allReqs, versionRequirement{feature: "Packet capture", minVersion: "4.16.0"})
	}

	for _, req := range allReqs {
		minVer, err := semver.NewVersion(req.minVersion)
		if err != nil {
			continue
		}
		if current.LessThan(minVer) {
			return fmt.Errorf("%s requires OpenShift %s or higher", req.feature, req.minVersion)
		}
	}
	return nil
}

// singularLabel replaces setup()'s hardcoded "creating flow-capture agents" / "packet-capture" /
// "metric-capture" progress messages, which used the singular form matching the underlying
// manifest filenames (flow-capture.yml, ...) rather than the plural CaptureType value itself.
func singularLabel(kind sharedcapture.Type) string {
	return kind.Singular()
}

func printYAMLInstructions(out io.Writer, kind sharedcapture.Type) {
	fmt.Fprintln(out, "Check the generated YAML file in output folder.")
	if kind.HasCollector() {
		fmt.Fprintln(out, "Apply it with: oc apply -f ./output/<file>.yml")
	} else if kind == sharedcapture.Metrics {
		fmt.Fprintln(out, "Apply it, then open your OCP Console and search for the netobserv-cli dashboard.")
	}
}
