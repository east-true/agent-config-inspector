package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/east-true/agent-config-inspector/internal/app"
	"github.com/east-true/agent-config-inspector/internal/provider/registry"
	"github.com/east-true/agent-config-inspector/internal/report"
	"github.com/east-true/agent-config-inspector/internal/usercontext"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const (
	exitOK          = 0
	exitFinding     = 1
	exitUsage       = 2
	exitIncomplete  = 3
	exitUnsupported = 4
	exitSafety      = 5
)

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }

func (values *stringList) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			*values = append(*values, item)
		}
	}
	return nil
}

type commandOptions struct {
	workspace          string
	format             string
	providers          stringList
	targets            stringList
	includeUserContext bool
	followSymlinks     bool
	maxSourceBytes     int64
	maxImportDepth     int
	failOn             string
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return exitUsage
	}
	scanner := app.New()
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "agent-config-inspector %s\n", agentconfig.Version)
		return exitOK
	case "providers":
		return runProviders(scanner, args[1:], stdout, stderr)
	case "scan", "explain", "diff":
		return runAnalysis(ctx, scanner, args[0], args[1:], stdout, stderr)
	case "matrix", "pin", "verify", "probe":
		fmt.Fprintf(stderr, "%s is planned but not implemented in this preview\n", args[0])
		return exitUnsupported
	case "help", "--help", "-h":
		writeUsage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		writeUsage(stderr)
		return exitUsage
	}
}

func runAnalysis(ctx context.Context, scanner *app.Scanner, command string, args []string, stdout, stderr io.Writer) int {
	options, err := parseAnalysisOptions(command, args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	if command == "explain" && len(options.providers) != 1 {
		fmt.Fprintln(stderr, "explain requires exactly one --provider")
		return exitUsage
	}
	if command == "diff" && len(options.providers) != 2 {
		fmt.Fprintln(stderr, "diff requires exactly two providers via --providers a,b")
		return exitUsage
	}
	if command != "scan" && len(options.targets) == 0 {
		options.targets = append(options.targets, ".")
	}
	result, scanErr := scanner.Scan(ctx, options.workspace, agentconfig.ScanOptions{
		Targets: options.targets, Providers: options.providers, IncludeUserContext: options.includeUserContext,
		FollowSymlinks: options.followSymlinks, MaxSourceBytes: options.maxSourceBytes, MaxImportDepth: options.maxImportDepth,
	})
	if scanErr != nil {
		var unsupported *registry.UnsupportedError
		if errors.As(scanErr, &unsupported) {
			fmt.Fprintln(stderr, scanErr)
			return exitUnsupported
		}
		var safety *usercontext.SafetyError
		if errors.As(scanErr, &safety) || errors.Is(scanErr, workspace.ErrOutsideWorkspace) || errors.Is(scanErr, workspace.ErrSymlink) {
			fmt.Fprintln(stderr, scanErr)
			return exitSafety
		}
		fmt.Fprintln(stderr, scanErr)
		return exitIncomplete
	}
	switch options.format {
	case "text":
		err = report.WriteText(stdout, result)
	case "json":
		err = report.WriteJSON(stdout, result)
	default:
		fmt.Fprintf(stderr, "unsupported format %q; use text or json\n", options.format)
		return exitUsage
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitIncomplete
	}
	if reachesThreshold(result.Findings, options.failOn) {
		return exitFinding
	}
	if !result.Complete {
		return exitIncomplete
	}
	return exitOK
}

func parseAnalysisOptions(command string, args []string, output io.Writer) (commandOptions, error) {
	options := commandOptions{workspace: ".", format: "text", maxSourceBytes: 1 << 20, maxImportDepth: 4, failOn: "error"}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		options.workspace = args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Var(&options.providers, "provider", "provider ID or alias; repeatable")
	flags.Var(&options.providers, "providers", "comma-separated provider IDs")
	flags.Var(&options.targets, "target", "workspace-relative target; repeatable")
	flags.Var(&options.targets, "targets", "comma-separated workspace-relative targets")
	flags.StringVar(&options.format, "format", options.format, "text or json")
	flags.BoolVar(&options.includeUserContext, "include-user-context", false, "opt in to redacted user-level instructions")
	flags.BoolVar(&options.followSymlinks, "follow-workspace-symlinks", false, "follow symlinks that remain inside workspace")
	flags.Int64Var(&options.maxSourceBytes, "max-source-bytes", options.maxSourceBytes, "maximum bytes read from one source")
	flags.IntVar(&options.maxImportDepth, "max-import-depth", options.maxImportDepth, "maximum Claude import hops, capped at 4")
	flags.StringVar(&options.failOn, "fail-on", options.failOn, "error, warning, or never")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() > 0 {
		return options, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	options.format = strings.ToLower(options.format)
	options.failOn = strings.ToLower(options.failOn)
	if options.maxSourceBytes <= 0 {
		return options, errors.New("--max-source-bytes must be positive")
	}
	if options.maxImportDepth <= 0 || options.maxImportDepth > 4 {
		return options, errors.New("--max-import-depth must be between 1 and 4")
	}
	if options.failOn != "error" && options.failOn != "warning" && options.failOn != "never" {
		return options, errors.New("--fail-on must be error, warning, or never")
	}
	return options, nil
}

func runProviders(scanner *app.Scanner, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 1 {
			fmt.Fprintln(stderr, "providers list takes no arguments")
			return exitUsage
		}
		for _, identity := range scanner.Registry.Identities() {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", identity.ID, identity.Provider, identity.Surface, identity.Support)
		}
		return exitOK
	}
	if args[0] == "show" {
		if len(args) != 2 {
			fmt.Fprintln(stderr, "providers show requires one provider ID")
			return exitUsage
		}
		adapter, err := scanner.Registry.Get(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitUnsupported
		}
		identity := adapter.Identity()
		fmt.Fprintf(stdout, "id: %s\nprovider: %s\nsurface: %s\nreported version: %s\nadapter: %s\nsupport: %s\ndepth: %s\n",
			identity.ID, identity.Provider, identity.Surface, identity.ReportedVersion, identity.AdapterID, identity.Support, identity.Depth)
		return exitOK
	}
	fmt.Fprintf(stderr, "unknown providers subcommand %q\n", args[0])
	return exitUsage
}

func reachesThreshold(findings []agentconfig.Finding, threshold string) bool {
	if threshold == "never" {
		return false
	}
	minimum := 2
	if threshold == "warning" {
		minimum = 1
	}
	for _, finding := range findings {
		level := map[string]int{"info": 0, "warning": 1, "error": 2}[finding.Severity]
		if level >= minimum {
			return true
		}
	}
	return false
}

func writeUsage(writer io.Writer) {
	lines := []string{
		"Agent Config Inspector predicts repository instructions for coding agents.",
		"",
		"Usage:",
		"  agent-config-inspector scan [workspace] [options]",
		"  agent-config-inspector explain [workspace] --provider <id> --target <path>",
		"  agent-config-inspector diff [workspace] --providers <a,b> --target <path>",
		"  agent-config-inspector providers list",
		"  agent-config-inspector providers show <id>",
		"  agent-config-inspector version",
		"",
		"Supported provider aliases: claude, codex",
	}
	for _, line := range lines {
		fmt.Fprintln(writer, line)
	}
}
