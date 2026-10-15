// Package manifests embeds the Kubernetes/OpenShift manifest templates this CLI deploys
// (res/*.yml, res/*.json — unchanged in content and location relative to the repo from the
// bash implementation's point of view, just moved under this package so go:embed can see them)
// and decodes them into typed objects. It replaces scripts/functions.sh's loadYAMLs, which
// read these same files from disk at runtime and did `{{PLACEHOLDER}}` string substitution.
package manifests

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

//go:embed res
var resFS embed.FS

// readTemplate loads an embedded file and substitutes every {{PLACEHOLDER}} the CLI's
// manifests use, replacing all occurrences (scripts/functions.sh mixed single- and
// all-occurrence bash substitutions; every current template has at most one distinct value per
// placeholder, so always replacing all occurrences is equivalent and simpler).
func readTemplate(path string, substitutions map[string]string) ([]byte, error) {
	data, err := resFS.ReadFile("res/" + path)
	if err != nil {
		return nil, fmt.Errorf("cannot read embedded manifest %q: %w", path, err)
	}
	content := string(data)
	for placeholder, value := range substitutions {
		content = strings.ReplaceAll(content, placeholder, value)
	}
	return []byte(content), nil
}

// decodeInto YAML-unmarshals a single-document template into a typed object. sigs.k8s.io/yaml
// round-trips through JSON, so TypeMeta (apiVersion/kind) is populated from the source document
// exactly as a real apiVersion/kind would be.
func decodeInto(data []byte, obj runtime.Object) error {
	if err := sigsyaml.Unmarshal(data, obj); err != nil {
		return fmt.Errorf("cannot decode manifest into %T: %w", obj, err)
	}
	return nil
}

// decodeAllUnstructured splits a multi-document YAML template (res/service-account.yml,
// res/service-monitor.yml) and decodes each document generically. These bundles are applied
// as-is and never field-edited at runtime, so there is no need for typed Go structs (which
// would require adding RBAC/SCC/ServiceMonitor API packages as dependencies just to round-trip
// values this tool never reads back).
func decodeAllUnstructured(data []byte) ([]runtime.Object, error) {
	reader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	var objs []runtime.Object
	for {
		doc, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read manifest document: %w", err)
		}
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}
		u := &unstructured.Unstructured{}
		if err := sigsyaml.Unmarshal(doc, &u.Object); err != nil {
			return nil, fmt.Errorf("cannot decode manifest document: %w", err)
		}
		objs = append(objs, u)
	}
	return objs, nil
}
