package k8sclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/utils/ptr"
	yamlutil "sigs.k8s.io/yaml"
)

// FieldManager identifies this tool's writes for Server-Side Apply, the way oc/kubectl use
// "kubectl-client-side-apply" or "oc-cmd" as their own field managers.
const FieldManager = "netobserv-cli"

// Sink is where rendered manifests go: either applied to a live cluster (ClusterSink) or
// written out as YAML (YAMLWriter, for --yaml). Both replace applyYAML's single `if
// outputYAML` branch with two small, independently testable implementations.
type Sink interface {
	Apply(ctx context.Context, objs ...runtime.Object) error
}

// ClusterSink applies objects to a live cluster via Server-Side Apply, replacing `oc apply -f -`.
// Server-Side Apply (rather than a client-side create-or-update) is deliberate: it is the
// idiomatic client-go equivalent of `kubectl apply`, and safe here because this tool is the sole
// owner of the objects it manages (Force is set so a stale local manifest always wins).
type ClusterSink struct {
	Client *Client
}

// Apply applies every object and only then reports any failures, matching `kubectl/oc apply -f -`
// on a multi-document file: it doesn't abort on the first unrecognized/invalid object, it applies
// everything it can and reports the rest as one aggregated error at the end, matching the
// multi-document behavior users expect from kubectl/oc apply.
func (s *ClusterSink) Apply(ctx context.Context, objs ...runtime.Object) error {
	mapper, err := s.restMapper()
	if err != nil {
		return err
	}
	var errs []error
	for _, obj := range objs {
		if err := s.applyOne(ctx, mapper, obj); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *ClusterSink) restMapper() (meta.RESTMapper, error) {
	groupResources, err := restmapper.GetAPIGroupResources(s.Client.Discovery)
	if err != nil {
		return nil, fmt.Errorf("cannot discover API group resources: %w", err)
	}
	return restmapper.NewDiscoveryRESTMapper(groupResources), nil
}

func (s *ClusterSink) applyOne(ctx context.Context, mapper meta.RESTMapper, obj runtime.Object) error {
	u, err := ToUnstructured(obj)
	if err != nil {
		return err
	}

	gvk := u.GroupVersionKind()
	if gvk.Empty() {
		return fmt.Errorf("object %q has no apiVersion/kind set", u.GetName())
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("cannot map %s: %w", gvk, err)
	}

	data, err := json.Marshal(u.Object)
	if err != nil {
		return fmt.Errorf("cannot marshal %s %q: %w", gvk.Kind, u.GetName(), err)
	}

	var resource dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns := u.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		resource = s.Client.Dynamic.Resource(mapping.Resource).Namespace(ns)
	} else {
		resource = s.Client.Dynamic.Resource(mapping.Resource)
	}

	_, err = resource.Patch(ctx, u.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: FieldManager,
		Force:        ptr.To(true),
	})
	if err != nil {
		return fmt.Errorf("cannot apply %s %q: %w", gvk.Kind, u.GetName(), err)
	}
	return nil
}

// YAMLWriter renders objects as `---`-separated YAML documents instead of applying them,
// replacing applyYAML's --yaml branch (which appended to ./output/<cmd>_capture_<date>.yml).
type YAMLWriter struct {
	w         io.Writer
	wroteOnce bool
}

func NewYAMLWriter(w io.Writer) *YAMLWriter {
	return &YAMLWriter{w: w}
}

func (y *YAMLWriter) Apply(_ context.Context, objs ...runtime.Object) error {
	for _, obj := range objs {
		u, err := ToUnstructured(obj)
		if err != nil {
			return err
		}
		data, err := yamlutil.Marshal(u.Object)
		if err != nil {
			return fmt.Errorf("cannot marshal %s %q to YAML: %w", u.GetKind(), u.GetName(), err)
		}
		if y.wroteOnce {
			if _, err := io.WriteString(y.w, "---\n"); err != nil {
				return err
			}
		}
		if _, err := y.w.Write(data); err != nil {
			return err
		}
		y.wroteOnce = true
	}
	return nil
}

// ToUnstructured converts a typed or already-unstructured runtime.Object into *unstructured.Unstructured.
func ToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u, nil
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("cannot convert %T to unstructured: %w", obj, err)
	}
	return &unstructured.Unstructured{Object: m}, nil
}
