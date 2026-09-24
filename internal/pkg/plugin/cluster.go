package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	// Register kubeconfig authentication providers, including OIDC.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/yaml"
)

// Like the operator's liveClient, cluster uses typed clients for Kubernetes and
// a dynamic client for platform APIs. No external Kubernetes CLI is invoked.
type cluster struct {
	kube      kubernetes.Interface
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
	mapper    meta.RESTMapper
	config    *rest.Config
}

func connect(o *options) (*cluster, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = o.kubeconfig
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: o.context}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	cfg.UserAgent = "oc-netobserv"
	k, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	d, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &cluster{kube: k, dynamic: d, discovery: dc, mapper: restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc)), config: cfg}, nil
}
func (c *cluster) isOpenShift() (bool, error) {
	groups, err := c.discovery.ServerGroups()
	if err != nil {
		return false, fmt.Errorf("discover cluster APIs: %w", err)
	}
	for i := range groups.Groups {
		if groups.Groups[i].Name == "config.openshift.io" {
			return true, nil
		}
	}
	return false, nil
}
func (c *cluster) create(ctx context.Context, obj *unstructured.Unstructured, owner *metav1.OwnerReference) error {
	mapping, err := c.mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
	if err != nil {
		return err
	}
	var resource dynamic.ResourceInterface = c.dynamic.Resource(mapping.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		resource = c.dynamic.Resource(mapping.Resource).Namespace(obj.GetNamespace())
	}
	obj.SetOwnerReferences([]metav1.OwnerReference{*owner})
	_, err = resource.Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// Older shell captures leave cluster-scoped RBAC behind. Reconcile those
		// resources just as kubectl apply did, preserving optimistic concurrency.
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current, getErr := resource.Get(ctx, obj.GetName(), metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			for _, ref := range current.GetOwnerReferences() {
				if ref.UID != owner.UID {
					return fmt.Errorf("%s %s belongs to another capture", obj.GetKind(), obj.GetName())
				}
			}
			obj.SetResourceVersion(current.GetResourceVersion())
			_, updateErr := resource.Update(ctx, obj, metav1.UpdateOptions{})
			return updateErr
		})
	}
	if err != nil {
		return fmt.Errorf("create %s %s: %w", obj.GetKind(), obj.GetName(), err)
	}
	return nil
}
func ignoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
func (c *cluster) stop(ctx context.Context, ns string) error {
	return ignoreNotFound(c.kube.AppsV1().DaemonSets(ns).Delete(ctx, "netobserv-cli", metav1.DeleteOptions{}))
}
func (c *cluster) cleanup(ctx context.Context, ns string) error {
	// All created resources, including cluster-scoped RBAC and the dashboard, are
	// owned by this namespace and garbage-collected with it.
	current, err := c.kube.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// The shell plugin allowed cleanup of the dedicated default namespace even
	// when it was created manually and did not have the capture label.
	if ns != "netobserv-cli" && current.Labels["app"] != "netobserv-cli" {
		return fmt.Errorf("refusing to delete namespace %s: it is not a capture namespace", ns)
	}
	dashboard, dashboardErr := c.kube.CoreV1().ConfigMaps("openshift-config-managed").Get(ctx, ns, metav1.GetOptions{})
	if dashboardErr == nil {
		owned := ns == "netobserv-cli" && len(dashboard.OwnerReferences) == 0
		for _, ref := range dashboard.OwnerReferences {
			owned = owned || ref.UID == current.UID
		}
		if owned {
			dashboardErr = c.kube.CoreV1().ConfigMaps("openshift-config-managed").Delete(ctx, ns, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &dashboard.UID}})
		}
	}
	if dashboardErr != nil && !apierrors.IsNotFound(dashboardErr) {
		return dashboardErr
	}
	uid := current.UID
	if err := ignoreNotFound(c.kube.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})); err != nil {
		return err
	}
	// Match oc delete's wait behavior so the next capture can reuse the namespace.
	return wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		_, err := c.kube.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		return apierrors.IsNotFound(err), ignoreNotFound(err)
	})
}
func (c *cluster) waitPod(ctx context.Context, ns string) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		p, err := c.kube.CoreV1().Pods(ns).Get(ctx, "collector", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if p.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("collector failed: %s", p.Status.Message)
		}
		for _, condition := range p.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}
func (c *cluster) waitAgents(ctx context.Context, ns string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		ds, err := c.kube.AppsV1().DaemonSets(ns).Get(ctx, "netobserv-cli", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return ds.Status.ObservedGeneration >= ds.Generation && ds.Status.DesiredNumberScheduled > 0 && ds.Status.NumberReady == ds.Status.DesiredNumberScheduled, nil
	})
}
func (c *cluster) follow(ctx context.Context, ns string, out io.Writer) error {
	stream, err := c.kube.CoreV1().Pods(ns).GetLogs("collector", &corev1.PodLogOptions{Container: "collector", Follow: true}).Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	_, err = io.Copy(out, stream)
	return err
}
func (c *cluster) exec(ctx context.Context, ns string, args []string, in io.Reader, out, errOut io.Writer, tty bool) error {
	request := c.kube.CoreV1().RESTClient().Post().Resource("pods").Namespace(ns).Name("collector").SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: "collector", Command: args, Stdin: in != nil, Stdout: out != nil, Stderr: errOut != nil && !tty, TTY: tty}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(c.config, http.MethodPost, request.URL())
	if err != nil {
		return err
	}
	websocket, err := remotecommand.NewWebSocketExecutor(c.config, http.MethodGet, request.URL().String())
	if err != nil {
		return err
	}
	fallback, err := remotecommand.NewFallbackExecutor(websocket, executor, func(err error) bool { return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err) })
	if err != nil {
		return err
	}
	if f, ok := in.(*os.File); ok {
		input := newExecInput(f)
		defer input.stop()
		in = input
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	return fallback.StreamWithContext(streamCtx, remotecommand.StreamOptions{Stdin: in, Stdout: out, Stderr: errOut, Tty: tty, TerminalSizeQueue: &terminalSize{ctx: streamCtx}})
}

type terminalSize struct {
	ctx      context.Context
	previous remotecommand.TerminalSize
}

func (q *terminalSize) Next() *remotecommand.TerminalSize {
	for {
		w, h, err := term.GetSize(int(os.Stdin.Fd()))
		if err != nil {
			return nil
		}
		size := remotecommand.TerminalSize{Width: uint16(w), Height: uint16(h)}
		if size != q.previous {
			q.previous = size
			return &size
		}
		select {
		case <-q.ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (c *cluster) subnets(ctx context.Context) ([]any, error) {
	cm, err := c.kube.CoreV1().ConfigMaps("kube-system").Get(ctx, "cluster-config-v1", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get machine networks: %w", err)
	}
	var install struct {
		Networking struct {
			MachineNetwork []struct {
				CIDR string `json:"cidr"`
			} `json:"machineNetwork"`
		} `json:"networking"`
	}
	if err = yaml.Unmarshal([]byte(cm.Data["install-config"]), &install); err != nil {
		return nil, err
	}
	machines := []any{}
	for _, network := range install.Networking.MachineNetwork {
		machines = append(machines, network.CIDR)
	}
	network, err := c.dynamic.Resource(schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "networks"}).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	pods, _, _ := unstructured.NestedSlice(network.Object, "spec", "clusterNetwork")
	podCIDRs := []any{}
	for _, p := range pods {
		if cidr, ok := p.(map[string]any)["cidr"]; ok {
			podCIDRs = append(podCIDRs, cidr)
		}
	}
	services, _, _ := unstructured.NestedSlice(network.Object, "spec", "serviceNetwork")
	result := []any{}
	for _, entry := range []struct {
		name  string
		cidrs []any
	}{{"Machines", machines}, {"Pods", podCIDRs}, {"Services", services}} {
		if len(entry.cidrs) > 0 {
			result = append(result, map[string]any{"name": entry.name, "cidrs": entry.cidrs})
		}
	}
	return result, nil
}
func (c *cluster) checkVersion(ctx context.Context, o *options) error {
	cv, err := c.dynamic.Resource(schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "clusterversions"}).Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return err
	}
	history, _, _ := unstructured.NestedSlice(cv.Object, "status", "history")
	for _, entry := range history {
		h := entry.(map[string]any)
		if h["state"] != "Completed" {
			continue
		}
		version, _ := h["version"].(string)
		var major, minor int
		if _, err := fmt.Sscanf(version, "%d.%d", &major, &minor); err != nil {
			return fmt.Errorf("invalid cluster version %q", version)
		}
		required := 0
		if o.mode == "packets" {
			required = 16
		}
		if o.enabled("enable_pkt_drop") && required < 14 {
			required = 14
		}
		if o.enabled("enable_udn_mapping") {
			required = 18
		}
		if o.enabled("enable_network_events") {
			required = 19
		}
		if major < 4 || major == 4 && minor < required {
			return fmt.Errorf("requested capture features require OpenShift 4.%d or newer (found %s)", required, version)
		}
		break
	}
	return nil
}
func (c *cluster) run(ctx context.Context, o *options, in io.Reader, out, errOut io.Writer) (result error) {
	m, err := c.prepareCapture(ctx, o)
	if err != nil {
		return err
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: o.namespace, Labels: m.objects[0].GetLabels()}}
	ns, err = c.kube.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create capture namespace (use cleanup for a previous capture): %w", err)
	}
	fmt.Fprintf(out, "namespace/%s created\n", ns.Name)
	owner := metav1.OwnerReference{APIVersion: "v1", Kind: "Namespace", Name: ns.Name, UID: ns.UID}
	keep := false
	started := false
	defer func() {
		if keep {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if started && o.mode != "metrics" && shouldCopy(o.copy, in, out) {
			if err := c.copyOutput(cleanupCtx, o.namespace, o.output, errOut); err != nil {
				result = errors.Join(result, fmt.Errorf("copy failed; capture retained, retry with copy: %w", err))
				return
			}
		}
		result = errors.Join(result, c.cleanup(cleanupCtx, o.namespace))
	}()
	if o.mode == "metrics" {
		port, err := c.reserveMetricsPort(ctx, &owner)
		if err != nil {
			return err
		}
		if err := m.setMetricsPort(port); err != nil {
			return err
		}
		fmt.Fprintf(out, "Reserved metrics host port %d\n", port)
	}
	if err = c.deployCapture(ctx, o, m, &owner, out); err != nil {
		return err
	}
	started = true
	if o.mode == "metrics" {
		console, consoleErr := c.dynamic.Resource(schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "consoles"}).Get(ctx, "cluster", metav1.GetOptions{})
		if consoleErr == nil {
			url, _, _ := unstructured.NestedString(console.Object, "status", "consoleURL")
			fmt.Fprintf(out, "Open %s/monitoring/dashboards/%s to see generated metrics.\n", strings.TrimSuffix(url, "/"), o.namespace)
		}
	}
	if o.background {
		keep = true
		fmt.Fprintln(out, "Capture started. Use oc-netobserv follow, copy, stop, or cleanup with the same namespace/context.")
		return nil
	}
	err = c.runCollector(ctx, o, in, out, errOut)
	if o.mode == "metrics" && err == nil && !o.headless {
		keep = true
		fmt.Fprintln(out, "Metrics remain available in the OpenShift NetObserv dashboard. Use stop or cleanup when finished.")
	}
	return err
}

func (c *cluster) runCollector(ctx context.Context, o *options, in io.Reader, out, errOut io.Writer) (result error) {
	tty := false
	if f, ok := in.(*os.File); ok && !o.headless && term.IsTerminal(int(f.Fd())) {
		state, e := term.MakeRaw(int(f.Fd()))
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, term.Restore(int(f.Fd()), state)) }()
		tty = true
	}
	execIn := in
	if o.headless {
		execIn = nil
	}
	err := c.exec(ctx, o.namespace, o.collectorCommand(), execIn, out, errOut, tty)
	if err != nil {
		err = fmt.Errorf("command terminated: %w", err)
	}
	return err
}

func (c *cluster) prepareCapture(ctx context.Context, o *options) (*manifests, error) {
	openshift, err := c.isOpenShift()
	if err != nil {
		return nil, err
	}
	if o.mode == "metrics" && !openshift {
		return nil, fmt.Errorf("metrics capture requires OpenShift monitoring")
	}
	if openshift {
		if err = c.checkVersion(ctx, o); err != nil {
			return nil, err
		}
	}
	var subnets []any
	if o.subnets {
		subnets, err = c.subnets(ctx)
		if err != nil {
			return nil, err
		}
	}
	m, err := buildManifests(o, openshift, subnets)
	if err != nil {
		return nil, err
	}
	if len(m.agent.Spec.Template.Spec.NodeSelector) > 0 {
		selectors := []string{}
		for k, v := range m.agent.Spec.Template.Spec.NodeSelector {
			selectors = append(selectors, k+"="+v)
		}
		nodes, err := c.kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: strings.Join(selectors, ",")})
		if err != nil {
			return nil, err
		}
		if len(nodes.Items) == 0 {
			return nil, fmt.Errorf("node selector matches no nodes")
		}
	}
	return m, nil
}

func (c *cluster) deployCapture(ctx context.Context, o *options, m *manifests, owner *metav1.OwnerReference, out io.Writer) error {
	var err error
	for _, obj := range m.objects[1:] {
		if err = c.create(ctx, obj, owner); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s/%s created\n", strings.ToLower(obj.GetKind()), obj.GetName())
	}
	m.pod.OwnerReferences = []metav1.OwnerReference{*owner}
	if _, err = c.kube.CoreV1().Pods(o.namespace).Create(ctx, m.pod, metav1.CreateOptions{}); err != nil {
		return err
	}
	fmt.Fprintln(out, "pod/collector created")
	fmt.Fprintln(out, "Waiting for collector...")
	if err = c.waitPod(ctx, o.namespace); err != nil {
		return fmt.Errorf("collector readiness: %w", err)
	}
	fmt.Fprintln(out, "pod/collector condition met")
	m.agent.OwnerReferences = []metav1.OwnerReference{*owner}
	if _, err = c.kube.AppsV1().DaemonSets(o.namespace).Create(ctx, m.agent, metav1.CreateOptions{}); err != nil {
		return err
	}
	fmt.Fprintln(out, "daemonset.apps/netobserv-cli created")
	fmt.Fprintln(out, "Waiting for capture agents...")
	err = c.waitAgents(ctx, o.namespace)
	if err != nil {
		return fmt.Errorf("agent readiness: %w", err)
	}
	return nil
}
