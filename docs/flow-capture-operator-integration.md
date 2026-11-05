# FlowCapture API in the NetObserv Operator

## Question

Could the NetObserv Operator expose a new `FlowCapture` API that deploys the components currently created by the Go CLI, and can the two repositories share the implementation?

## Short answer

This is feasible, but it is not a matter of importing the CLI command package into the operator.

The recommended model is:

1. The operator owns a namespaced `FlowCapture` custom resource and reconciles its capture resources.
2. The CLI remains a direct deployer and keeps its current apply, wait, attach, copy, and cleanup workflow.
3. The operator and CLI can share stable, deployment-neutral builders hosted in the operator repository, without sharing the `FlowCapture` API or controller lifecycle.
4. Operator reconciliation, status management, ownership, cleanup, and CLI interaction remain repository-specific.

This gives the operator a supported API without turning the CLI into a dependency of the operator or duplicating the entire operator implementation in the CLI.

## What exists today

The CLI now has three distinct layers:

- `internal/plugin/commands` parses CLI flags and implements the interactive workflow.
- `internal/plugin/manifests` and `internal/plugin/manifests/agent` build capture-specific Kubernetes objects and eBPF/flowlogs-pipeline configuration.
- `internal/plugin/capture` applies, waits for, execs into, copies from, and cleans up the resources.

The CLI currently creates a namespace, RBAC, an eBPF-agent DaemonSet, a collector Pod and Service for flows/packets, or a ServiceMonitor for metrics. It uses fixed names such as `netobserv-cli`, `netobserv-ebpf-agent`, and `collector`, and deletes the capture resources when the foreground command exits.

The operator has a different model. Its existing `FlowCollector` is a single cluster-wide resource named `cluster`; it reconciles the long-running NetObserv installation, including the eBPF agent, flowlogs-pipeline, permissions, monitoring, and status. The eBPF controller already translates the `FlowCollector` API into agent environment variables, flow-filter JSON, security settings, volumes, and a DaemonSet. It also uses owned resources and component status rather than one-shot apply/delete operations.

Relevant references:

- [CLI capture command orchestration](../internal/plugin/commands/run.go)
- [CLI capture manifests](../internal/plugin/manifests)
- [CLI capture lifecycle](../internal/plugin/capture)
- [Operator FlowCollector API](https://github.com/netobserv/netobserv-operator/blob/main/api/flowcollector/v1beta2/flowcollector_types.go)
- [Operator FlowCollector reconciler](https://github.com/netobserv/netobserv-operator/blob/main/internal/controller/flowcollector_controller.go)
- [Operator eBPF agent controller](https://github.com/netobserv/netobserv-operator/blob/main/internal/controller/ebpf/agent_controller.go)
- [Operator namespaced resource manager](https://github.com/netobserv/netobserv-operator/blob/main/internal/controller/reconcilers/namespaced_objects_manager.go)

## Can the operator reuse the CLI code as-is?

No, not as-is.

### Go package boundaries

The CLI implementation is under an `internal` tree. Go prevents the operator module from importing packages under `github.com/netobserv/network-observability-cli/internal/...`. Moving the packages out of `internal` would technically make imports possible, but importing the CLI module would still couple the operator to command-oriented code and its dependency graph.

The CLI also contains Cobra commands, terminal prompts, `oc exec`/copy behavior, local output conversion, and process-oriented cleanup. None of those belong in an operator controller.

### Different ownership and lifecycle semantics

The CLI does this:

```text
build objects -> apply objects -> wait -> exec into collector -> copy -> delete
```

An operator must continuously do this:

```text
observe desired FlowCapture -> reconcile owned objects -> update status -> retry on events
```

The operator needs owner references or an equivalent ownership mechanism, idempotent create/update/delete behavior, finalizers where necessary, watches, status conditions, conflict handling, and recovery after operator or node restarts. Reusing the CLI's `Apply`, wait, exec, and cleanup workflow would fight controller-runtime instead of helping it.

### Resource identity

The CLI can use one fixed `netobserv-cli` namespace because it is an interactive, single-capture tool. A namespaced API must support at least multiple captures over time and should not assume that one capture owns the whole namespace. Resources need names and labels derived from the `FlowCapture` name, for example:

```text
netobserv-flowcapture-<name>-agents
netobserv-flowcapture-<name>-collector
```

The operator must never delete the shared operator namespace or resources belonging to another capture.

### Configuration coverage

The CLI's capture manifests are intentionally narrow and capture-oriented. The operator's eBPF controller already supports more production concerns, including related images, vendor-specific permissions, advanced scheduling, resource settings, certificate references, eBPF Manager, metrics, and feature validation. Copying the CLI templates into the operator would create a second implementation of agent deployment and quickly diverge.

The packet-capture path is particularly different: the operator primarily manages continuous flow extraction, while CLI packet capture has a collector/TLS/interactive-output workflow. Metrics capture also has a different lifecycle from the operator's continuous agent metrics and ServiceMonitor management.

## Proposed `FlowCapture` API

`FlowCapture` should be a namespaced, short-lived API in the operator's `flows.netobserv.io` group, initially `v1alpha1`:

```yaml
apiVersion: flows.netobserv.io/v1alpha1
kind: FlowCapture
metadata:
  name: example
  namespace: netobserv
spec:
  type: flows # flows, packets, or metrics
  maxTime: 5m
  maxBytes: 50000000
  agent:
    sampling: 1
    interfaces: [br-ex]
    excludeInterfaces: [lo]
    features: [DNSTracking]
    flowFilter:
      rules:
      - action: Accept
        protocol: TCP
        destPorts: 8080
  output:
    collectorImage: quay.io/netobserv/network-observability-cli:main
```

The exact schema needs to be designed with the operator maintainers, but these principles are important:

- Use typed fields rather than a bag of CLI arguments.
- Reuse existing `FlowCollector` agent/filter types where their semantics are identical, or define a shared neutral type if the APIs must evolve independently.
- Make `type` an explicit enum and validate fields by type. For example, `maxBytes` is meaningful for flows and packets, but not metrics.
- Make capture resource names deterministic from the `FlowCapture` name.
- Keep images and pull policies under operator configuration or explicit, validated fields; do not copy CLI build-time defaults into the API.
- Define retention/deletion behavior explicitly, such as `ttlAfterFinished` and whether output is retained after the CR is deleted.
- Include status conditions and resource references so a client can attach without parsing logs.

A useful status shape would expose:

```yaml
status:
  phase: Running # Pending, Running, Succeeded, Failed, Deleting
  conditions: []
  collector:
    podName: netobserv-flowcapture-example-collector
    ready: true
  agents:
    desired: 3
    ready: 3
  startedAt: "..."
  finishedAt: "..."
```

The CLI could then replace most of its current readiness scraping with a typed watch on the `FlowCapture` status.

## Recommended controller design

Add a dedicated `FlowCapture` controller rather than extending the cluster-wide `FlowCollector` reconciliation path with short-lived special cases.

The controller would:

1. Validate the capture type and typed options.
2. Resolve images and cluster/vendor capabilities through the operator's existing configuration and discovery code.
3. Build the desired ServiceAccount, Roles/ClusterRoles, DaemonSet, collector Pod/Service, ConfigMaps, and optional ServiceMonitor.
4. Attach owner references to namespaced resources and use labels such as `flows.netobserv.io/flowcapture=<name>`.
5. Reconcile with the operator's existing owned-resource helpers.
6. Update `Pending`, `Running`, `Succeeded`, and `Failed` status conditions.
7. Stop the capture when duration or max-byte limits are reached, or when the `FlowCapture` is deleted.
8. Remove only resources owned by that `FlowCapture`.

The operator workflow would be approximately:

```text
observe FlowCapture -> reconcile capture resources -> update status -> stop on completion/deletion
```

The CLI does not need to create or watch `FlowCapture`. It continues to deploy its own capture directly. The two paths can use the same resource builders, with different resource identities and lifecycle adapters.

## What should be shared?

### Share now: upstream component contracts

Both repositories should continue to use the typed configuration packages already owned by the components:

- `netobserv-ebpf-agent/pkg/config` for eBPF flow-filter semantics.
- `flowlogs-pipeline/pkg/config` for pipeline configuration.

These are already the right sharing boundary because they describe component behavior, not how a CLI or operator manages Kubernetes resources.

### Potentially share: pure desired-object builders from the operator repository

Because CLI and operator releases are already coupled, a new shared repository is not necessary. Move the common, lifecycle-neutral builders into a public package in the operator repository. A possible package boundary is:

```text
github.com/netobserv/netobserv-operator/pkg/capture
  options.go    neutral capture options and validation types
  agent.go      agent env/filter/pipeline configuration
  resources.go  typed desired-object builders
```

The package should:

- accept an explicit resource identity, namespace, images, output target, and capture options;
- return typed Kubernetes objects or a resource bundle;
- contain no Cobra, controller-runtime client, `oc`/`kubectl` execution, waiting, status writes, prompts, or cleanup;
- avoid importing either repository's `internal` packages;
- have its own compatibility and release policy.

The CLI can import this public package at the matching operator version, but it does not need to import or use `api/flowcapture`. The operator's `FlowCapture` controller and the CLI's direct deployment path can both call the same builders and then apply different lifecycle behavior.

The operator would add owner references, reconcile the returned objects, and publish status. The CLI would apply them, wait, exec, copy, and delete them. This removes duplicated object construction while keeping lifecycle behavior separate.

### Do not share: controller and terminal orchestration

Do not put these in the shared package:

- Cobra flag parsing and CLI defaults.
- Kubernetes apply/update/delete orchestration.
- controller-runtime reconciliation and watches.
- operator status conditions and finalizers.
- TUI, exec, copy, prompts, or local JSON conversion.
- fixed resource names or assumptions about a dedicated namespace.

Sharing these would make both projects harder to evolve and would force every CLI feature to look like an operator feature, or vice versa.

## Coupling and feature workflow

The goal should be to avoid this workflow:

```text
update config repository -> update operator -> update CLI -> update manifests
```

Instead, use ownership by concern:

| Concern | Owner | Consumers |
| --- | --- | --- |
| eBPF agent runtime/env contract | eBPF agent repository | operator, CLI/shared builders |
| FlowLogs-Pipeline config contract | flowlogs-pipeline repository | operator, CLI/shared builders |
| Long-running cluster installation | operator | users and CLI status/attach code |
| Short-lived capture API and lifecycle | operator | operator clients |
| Interactive direct deployment and local export | CLI | users |
| Shared capture resource construction | operator public package | operator, CLI |

Adding a new agent feature should require changing the agent contract and the consumers that expose it. It should not require updating an unrelated configuration repository merely to pass a flag through. A shared package should be introduced only when both consumers genuinely need the same behavior; it should not become a mandatory central registry for every feature.

## Suggested implementation phases

### Phase 1: prove the API without sharing repositories

- Add `FlowCapture` as `v1alpha1` in the operator.
- Support `flows` only.
- Reuse the operator's existing eBPF configuration and permissions code.
- Add a dedicated collector/transport implementation in the operator controller.
- Use owner references, status conditions, and deterministic per-capture names.
- Keep the CLI's current direct deployment path unchanged.

This phase answers the product questions: should captures be namespaced, can multiple captures coexist, where should output live, and should the operator own the collector Pod?

### Phase 2: extract shared builders

- Move only duplicated, pure resource/configuration builders into `operator/pkg/capture`.
- Update the CLI to use those public builders while retaining its current apply/wait/exec/copy/cleanup workflow.
- Have the operator `FlowCapture` controller use the same builders with owner references and operator-specific names.

### Phase 3: add packets and metrics

Add packet and metrics capture only after their lifecycle and output contracts are clear. They should not be forced into the same implementation if their resource graphs or status semantics differ.

## Effort and risk

This is a medium-sized operator feature, not a small wrapper around the existing CLI.

- A flows-only API using the operator's existing eBPF controller patterns is a reasonable first increment.
- Supporting packets, metrics, multi-capture isolation, status, cleanup, CLI attach, and compatibility tests is substantially larger.
- Extracting a shared package before the API shape stabilizes adds design and release overhead.

The main technical risks are resource identity, permissions/SCC behavior across Kubernetes and OpenShift, output persistence, capture completion detection, and avoiding conflicts with the existing cluster-wide `FlowCollector` installation.

## Recommendation

Build `FlowCapture` in the operator as a dedicated, namespaced, short-lived controller API. Keep the CLI as a direct deployer. Since releases are already coupled, place only the common desired-resource/configuration builders in a public `operator/pkg/capture` package and let both paths retain their own lifecycle logic.

Do not import the CLI implementation into the operator and do not copy the operator's reconciliation code into the CLI. Start by reusing the operator's existing eBPF configuration and permission logic. Extract a neutral shared resource/configuration package only after the first flows implementation demonstrates stable duplication.

This provides an API that other clients can use, keeps the operator authoritative for cluster resources, preserves the CLI's TUI strengths, and limits future feature work to the repository that owns each concern.

## References

- [NetObserv Operator](https://github.com/netobserv/netobserv-operator)
- [Operator FlowCollector API documentation](https://github.com/netobserv/netobserv-operator/blob/main/docs/FlowCollector.md)
- [Operator generated FlowCollector API reference](https://github.com/netobserv/netobserv-operator/blob/main/docs/flowcollector-flows-netobserv-io-v1beta2.adoc)
- [Operator agent controller](https://github.com/netobserv/netobserv-operator/blob/main/internal/controller/ebpf/agent_controller.go)
- [Operator controller architecture](https://github.com/netobserv/netobserv-operator/blob/main/internal/controller/flowcollector_controller.go)
- [NetObserv eBPF agent](https://github.com/netobserv/netobserv-ebpf-agent)
