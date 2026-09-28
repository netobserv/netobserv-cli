package plugin

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Help text preserves the Bash CLI reference, including all existing options.
//
//go:embed help/*.txt
var helpFiles embed.FS

func envDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// NewCommand creates the standalone plugin. Parsing is ordered because `or`
// separates filter groups and repeated flags apply to the current group.
func NewCommand(version string) *cobra.Command {
	c := &cobra.Command{Use: "netobserv", DisableFlagParsing: true, SilenceUsage: true, SilenceErrors: true}
	c.SetIn(os.Stdin)
	c.SetOut(os.Stdout)
	c.SetErr(os.Stderr)
	c.RunE = func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return printHelp(c.OutOrStdout(), "root", nil)
		}
		mode := strings.TrimPrefix(args[0], "--")
		if mode == "serve" {
			server := newServeCommand()
			server.SetArgs(args[1:])
			server.SetOut(c.OutOrStdout())
			server.SetErr(c.ErrOrStderr())
			return server.ExecuteContext(c.Context())
		}
		if mode == "version" {
			fmt.Fprintf(c.OutOrStdout(), "NetObserv CLI version %s\n", version)
			return nil
		}
		if mode == "help" {
			return printHelp(c.OutOrStdout(), "root", nil)
		}
		if !oneOf(mode, "flows", "packets", "metrics", "follow", "stop", "copy", "cleanup") {
			return fmt.Errorf("unknown command %s. Use 'netobserv help' to display options", args[0])
		}
		for _, arg := range args[1:] {
			if strings.HasSuffix(arg, "help") {
				return printHelp(c.OutOrStdout(), mode, args[1:])
			}
		}
		o, err := parseOptions(mode, args[1:])
		if err != nil {
			return err
		}
		if err = confirmPlaintext(o, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr()); err != nil {
			return err
		}
		if o.yaml && oneOf(mode, "flows", "packets", "metrics") {
			return writeCaptureYAML(c, o)
		}
		cl, err := connect(o)
		if err != nil {
			return err
		}
		switch mode {
		case "follow":
			return cl.follow(c.Context(), o.namespace, c.OutOrStdout())
		case "stop":
			return cl.stop(c.Context(), o.namespace)
		case "copy":
			return cl.copyOutput(c.Context(), o.namespace, o.output, c.ErrOrStderr())
		case "cleanup":
			return cl.cleanup(c.Context(), o.namespace)
		default:
			return cl.run(c.Context(), o, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr())
		}
	}
	return c
}

func printHelp(w io.Writer, mode string, args []string) error {
	b, err := helpFiles.ReadFile("help/" + mode + ".txt")
	if err != nil {
		return err
	}
	text := string(b)
	// Retain contextual highlighting when help is requested alongside flags.
	for _, arg := range args {
		if strings.HasSuffix(arg, "help") || arg == "or" {
			continue
		}
		key, _, _ := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if key != "" {
			text = strings.ReplaceAll(text, key, "\033[1;36m"+key+"\033[0m")
		}
	}
	_, err = io.WriteString(w, text+"\nConnection options: --kubeconfig=PATH --context=NAME --namespace=NAME\nOutput location: --output-dir=PATH (default: output)\n")
	return err
}
func shouldCopy(policy string, in io.Reader, out io.Writer) bool {
	if policy == "true" || os.Getenv("isE2E") == "true" {
		return true
	}
	if policy == "false" {
		return false
	}
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "Copy the capture output locally? [yes/no] ")
		if !scanner.Scan() {
			return false
		}
		answer := strings.ToLower(scanner.Text())
		if strings.HasPrefix(answer, "y") {
			return true
		}
		if strings.HasPrefix(answer, "n") {
			return false
		}
		fmt.Fprintln(out, "Please answer yes or no.")
	}
}

// shellQuote is used only for printed deployment instructions, never execution.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func writeCaptureYAML(c *cobra.Command, o *options) error {
	mode := o.mode
	var subnets []any
	if o.subnets {
		cl, err := connect(o)
		if err != nil {
			return err
		}
		subnets, err = cl.subnets(c.Context())
		if err != nil {
			return err
		}
	}
	m, err := buildManifests(o, false, subnets)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(o.output, 0700); err != nil {
		return err
	}
	name := filepath.Join(o.output, mode+"_capture_"+envDefault("dateName", time.Now().Format("2006_01_02_03_04"))+".yml")
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	writeErr := m.writeYAML(f)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Fprintf(c.OutOrStdout(), "Check the generated YAML file: %s\n", name)
	cli := "kubectl"
	if strings.HasPrefix(filepath.Base(os.Args[0]), "oc-") {
		cli = "oc"
	}
	fmt.Fprintf(c.OutOrStdout(), "You can create %s agents by executing:\n %s apply -f %s\n", mode, cli, shellQuote(name))
	if mode == "metrics" {
		fmt.Fprintln(c.OutOrStdout(), "Then open your OCP Console and search for netobserv-cli dashboard")
		return nil
	}
	podJSON, err := json.Marshal(m.pod)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.OutOrStdout(), "Then create the collector using:\n %s run -n %s collector --image=%s --restart=Never --overrides=%s\n", cli, shellQuote(o.namespace), shellQuote(m.pod.Spec.Containers[0].Image), shellQuote(string(podJSON)))
	fmt.Fprintf(c.OutOrStdout(), "Follow its progression with:\n %s logs collector -n %s -f\n", cli, shellQuote(o.namespace))
	return nil
}
