# Legacy CLI compatibility fixtures

The YAML manifests and `legacy-*.txt` flag lists were saved from the Bash
implementation before it was removed. Go compatibility tests in
`internal/pkg/plugin/compatibility_test.go` use them as independent baselines.

Keep these files in version control. Do not regenerate them from the Go code
under test: that would hide compatibility regressions. Review any intentional
baseline changes alongside the behavior change. The former Bash implementation
remains available in Git history.

The packet fixture includes the four disabled/default TLS environment entries
added to the upstream packet template by NETOBSERV-2858 (#540). Existing
filter and security expectations remain unchanged.
