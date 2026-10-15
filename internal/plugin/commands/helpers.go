package commands

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
	"github.com/netobserv/network-observability-cli/internal/k8s/ocp"
	"github.com/netobserv/network-observability-cli/internal/plugin/capture"
	"github.com/netobserv/network-observability-cli/internal/plugin/manifests"
)

// stdin is overridden in tests so promptYesNo doesn't block on the real terminal.
var stdin io.Reader = os.Stdin

// newestTxtFile replaces `find ./output -name "*txt" | sort | tail -1`: a recursive search (the
// collector writes its flow/packet dumps into an output/flow/ or output/pcap/ subdirectory, not
// directly under output/), returning the lexicographically-last match (collector filenames are
// timestamp-prefixed, so that's also the most recent one).
func newestTxtFile(dir string) (string, error) {
	var names []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(d.Name()) == ".txt" {
			names = append(names, path)
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if len(names) == 0 {
		return "", nil
	}
	sort.Strings(names)
	return names[len(names)-1], nil
}

// checkNodeSelectorMatches replaces getNodesByLabel: fail fast if --node-selector matches no
// node, rather than silently deploying a DaemonSet that will never schedule anywhere.
func checkNodeSelectorMatches(ctx context.Context, client *k8sclient.Client, nodeSelector string, out io.Writer) error {
	key, value := splitNodeSelector(nodeSelector)
	fmt.Fprintf(out, "finding nodes matching %s=%s...\n", key, value)
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", key, value),
	})
	if err != nil {
		return fmt.Errorf("cannot list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return fmt.Errorf("%s=%s doesn't match any node; check your --node-selector", key, value)
	}
	fmt.Fprintf(out, "%d matching node(s) found\n", len(nodes.Items))
	return nil
}

// buildOptionString replaces commands/netobserv's optionStr construction: every raw argument
// passed to the subcommand (filters, "or", enable_* flags, everything — scripts/functions.sh's
// options=("$@") captured the whole thing), "|"-joined with each leading "--" stripped, forwarded
// to the in-pod CLI's own --options flag (which configures its TUI/display layer; the DaemonSet
// env vars set by internal/plugin/manifests/agent separately configure the eBPF agent itself).
func buildOptionString(rawArgs []string) string {
	if len(rawArgs) == 0 {
		return ""
	}
	parts := make([]string, len(rawArgs))
	for i, a := range rawArgs {
		parts[i] = strings.TrimPrefix(a, "--")
	}
	return strings.Join(parts, "|")
}

// buildInPodArgs replaces commands/netobserv's execCommand/execCommandBase construction: the
// in-pod CLI invocation, used both as the foreground `oc exec` command and (wrapped in `bash -c
// ... && sleep infinity`) as the background Pod's own startup command.
func buildInPodArgs(kind manifests.CaptureKind, capOpts *captureOptions, namespace, optionStr string) []string {
	args := []string{
		"/network-observability-cli", "get-" + string(kind),
		"--loglevel", capOpts.logLevel,
		"--maxtime", capOpts.maxTime.String(),
		"--namespace", namespace,
	}
	if kind.HasCollector() {
		args = append(args, "--maxbytes", strconv.FormatInt(capOpts.maxBytes, 10))
	}
	if optionStr != "" {
		args = append(args, "--options", optionStr)
	}
	return args
}

// shellQuoteJoin renders args as a POSIX shell command line, for the `bash -c "<cmd> && sleep
// infinity"` background Pod command.
func shellQuoteJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

func printBackgroundInstructions(ctx context.Context, out io.Writer, kind manifests.CaptureKind, capOpts *captureOptions, client *k8sclient.Client) {
	switch kind {
	case manifests.Flows, manifests.Packets:
		fmt.Fprintln(out, "Background capture started. Use:")
		fmt.Fprintln(out, " - 'netobserv follow' to see the capture progress")
		fmt.Fprintln(out, " - 'netobserv copy' to copy the generated files locally")
		fmt.Fprintln(out, " - 'netobserv cleanup' to remove the netobserv components")
		fmt.Fprintf(out, "\nCapture ends automatically in %s or reaching %d bytes\n", capOpts.maxTime, capOpts.maxBytes)
	case manifests.Metrics:
		fmt.Fprintln(out, "Metrics capture started.")
		if url, err := ocp.ConsoleURL(ctx, client.Dynamic); err == nil && url != "" {
			fmt.Fprintf(out, "Open %s/monitoring/dashboards/netobserv-cli to see generated metrics.\n", url)
		}
		fmt.Fprintln(out, "Use 'netobserv stop' to stop the collection and 'netobserv cleanup' to remove everything.")
		fmt.Fprintf(out, "\nCapture ends automatically in %s\n", capOpts.maxTime)
	}
}

// onCaptureExit replaces cleanup()'s trap-on-EXIT behavior after an interactive foreground
// capture: prompt to copy the output, then tear every resource down.
func onCaptureExit(ctx context.Context, client *k8sclient.Client, namespace string, kind manifests.CaptureKind, capOpts *captureOptions, out io.Writer) error {
	if kind.HasCollector() {
		// isE2E replaces bash's `if [[ "$isE2E" = true || "$copy" = true ]]; then copyOutput; fi`:
		// the e2e test harness sets this to skip the interactive prompt and always copy, regardless
		// of --copy or how the capture ended (including a signal-interrupted one).
		switch {
		case os.Getenv("isE2E") == "true", capOpts.copyMode == "true":
			if err := doCopy(ctx, client, namespace, out); err != nil {
				fmt.Fprintf(out, "copy failed: %v\n", err)
			}
		case capOpts.copyMode == "prompt":
			if promptYesNo(out, "Copy the capture output locally? [yes/no] ") {
				if err := doCopy(ctx, client, namespace, out); err != nil {
					fmt.Fprintf(out, "copy failed: %v\n", err)
				}
			} else {
				fmt.Fprintln(out, "copy skipped")
			}
		default:
			fmt.Fprintln(out, "copy skipped")
		}
	}

	fmt.Fprintln(out, "\nCleaning up...")
	return capture.Cleanup(ctx, client, namespace, out)
}

// doCopy replaces copyOutput: copy the collector's /output directory locally, then convert the
// newest flow-log text dump into JSON (see capture.BuildJSON for why the last line is dropped).
func doCopy(ctx context.Context, client *k8sclient.Client, namespace string, out io.Writer) error {
	fmt.Fprintln(out, "Copying collector files to ./output...")
	if err := capture.CopyFromPod(ctx, client, namespace, capture.CollectorPodName, "/output", "./output", out); err != nil {
		return err
	}
	flowFile, err := newestTxtFile("./output")
	if err != nil || flowFile == "" {
		return nil //nolint:nilerr // no .txt dump present (e.g. a metrics capture) is not an error
	}
	if _, err := capture.BuildJSON(flowFile); err != nil {
		return err
	}
	return os.Remove(flowFile)
}

// promptYesNo replaces cleanup()'s `read -rp "... [yes/no] "` loop: EOF (non-interactive stdin,
// e.g. in e2e tests) is treated as "no" rather than looping forever.
func promptYesNo(out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	scanner := bufio.NewScanner(stdin)
	for scanner.Scan() {
		switch cleanPromptAnswer(scanner.Text()) {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		default:
			fmt.Fprint(out, "Please answer yes or no. ")
		}
	}
	return false
}

var terminalEscape = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\a]*(?:\a|$))`)

func cleanPromptAnswer(answer string) string {
	answer = terminalEscape.ReplaceAllString(answer, "")
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(answer)))
}
