package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/east-true/agent-config-inspector/internal/app"
	"github.com/east-true/agent-config-inspector/internal/probe"
	"github.com/east-true/agent-config-inspector/internal/provider/registry"
	"github.com/east-true/agent-config-inspector/internal/report"
	"github.com/east-true/agent-config-inspector/internal/skills"
	"github.com/east-true/agent-config-inspector/internal/snapshot"
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
	output             string
	snapshot           string
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
	case "pin":
		return runPin(ctx, scanner, args[1:], stdout, stderr)
	case "verify":
		return runVerify(ctx, scanner, args[1:], stdout, stderr)
	case "probe":
		return runProbe(ctx, scanner, args[1:], stdout, stderr)
	case "inventory":
		return runInventory(ctx, scanner, args[1:], stdout, stderr)
	case "matrix":
		fmt.Fprintln(stderr, "matrix is planned but not implemented in this preview")
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

func runInventory(ctx context.Context, scanner *app.Scanner, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "skills" {
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			writeInventoryUsage(stdout)
			return exitOK
		}
		fmt.Fprintln(stderr, "inventory currently requires the skills surface")
		writeInventoryUsage(stderr)
		return exitUsage
	}
	options, err := parseInventoryOptions(args[1:], stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	service := skills.New(scanner.Registry)
	reportValue, inventoryErr := service.Inventory(ctx, options.workspace, agentconfig.SkillInventoryOptions{
		Targets: options.targets, Providers: options.providers, FollowSymlinks: options.followSymlinks, MaxSourceBytes: options.maxSourceBytes,
	})
	if inventoryErr != nil {
		var unsupportedInventory *skills.UnsupportedError
		var unsupportedProvider *registry.UnsupportedError
		if errors.As(inventoryErr, &unsupportedInventory) || errors.As(inventoryErr, &unsupportedProvider) {
			fmt.Fprintln(stderr, inventoryErr)
			return exitUnsupported
		}
		if errors.Is(inventoryErr, workspace.ErrOutsideWorkspace) || errors.Is(inventoryErr, workspace.ErrSymlink) {
			fmt.Fprintln(stderr, inventoryErr)
			return exitSafety
		}
		fmt.Fprintln(stderr, inventoryErr)
		return exitIncomplete
	}
	if options.format == "json" {
		err = skills.WriteJSON(stdout, reportValue)
	} else {
		err = skills.WriteText(stdout, reportValue)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitIncomplete
	}
	if reachesThreshold(reportValue.Findings, options.failOn) {
		return exitFinding
	}
	if !reportValue.Complete {
		return exitIncomplete
	}
	return exitOK
}

func parseInventoryOptions(args []string, output io.Writer) (commandOptions, error) {
	options := commandOptions{workspace: ".", format: "text", maxSourceBytes: 1 << 20, failOn: "error"}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		options.workspace = args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet("inventory skills", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Var(&options.providers, "provider", "provider ID or alias; repeatable (claude and codex only)")
	flags.Var(&options.providers, "providers", "comma-separated provider IDs (claude and codex only)")
	flags.Var(&options.targets, "target", "workspace-relative launch path; repeatable")
	flags.Var(&options.targets, "targets", "comma-separated workspace-relative launch paths")
	flags.StringVar(&options.format, "format", options.format, "text or json")
	flags.BoolVar(&options.followSymlinks, "follow-workspace-symlinks", false, "follow skill symlinks that remain inside workspace")
	flags.Int64Var(&options.maxSourceBytes, "max-source-bytes", options.maxSourceBytes, "maximum bytes read from one SKILL.md")
	flags.StringVar(&options.failOn, "fail-on", options.failOn, "error, warning, or never")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() > 0 {
		return options, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	options.format = strings.ToLower(options.format)
	options.failOn = strings.ToLower(options.failOn)
	if options.format != "text" && options.format != "json" {
		return options, errors.New("--format must be text or json")
	}
	if options.maxSourceBytes <= 0 {
		return options, errors.New("--max-source-bytes must be positive")
	}
	if options.failOn != "error" && options.failOn != "warning" && options.failOn != "never" {
		return options, errors.New("--fail-on must be error, warning, or never")
	}
	return options, nil
}

func runProbe(ctx context.Context, scanner *app.Scanner, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "probe requires one provider ID or alias")
		writeProbeUsage(stderr)
		return exitUsage
	}
	if args[0] == "--help" || args[0] == "-h" {
		writeProbeUsage(stdout)
		return exitOK
	}
	adapter, err := scanner.Registry.Get(args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUnsupported
	}

	providerID := adapter.Identity().ID
	caseID := probe.DefaultCaseID
	format := "text"
	timeout := 2 * time.Minute
	execute := false
	acknowledgeQuota := false
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&caseID, "case", caseID, "behavioral case ID")
	flags.StringVar(&format, "format", format, "text or json")
	flags.DurationVar(&timeout, "timeout", timeout, "provider execution timeout")
	flags.BoolVar(&execute, "execute", false, "run the provider CLI and make a model request")
	flags.BoolVar(&acknowledgeQuota, "acknowledge-quota", false, "confirm that the model request may consume quota")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected positional arguments: %s\n", strings.Join(flags.Args(), " "))
		return exitUsage
	}
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintln(stderr, "--format must be text or json")
		return exitUsage
	}
	if timeout < 10*time.Second || timeout > 10*time.Minute {
		fmt.Fprintln(stderr, "--timeout must be between 10s and 10m")
		return exitUsage
	}
	if acknowledgeQuota && !execute {
		fmt.Fprintln(stderr, "--acknowledge-quota is only valid with --execute")
		return exitUsage
	}

	service := probe.New()
	plan, planErr := service.Plan(providerID, caseID, timeout)
	if planErr != nil {
		fmt.Fprintln(stderr, planErr)
		return exitUnsupported
	}
	if !execute {
		if err := probe.WritePlan(stdout, plan, format); err != nil {
			fmt.Fprintln(stderr, err)
			return exitIncomplete
		}
		return exitOK
	}
	if err := probe.WritePlan(stderr, plan, "text"); err != nil {
		fmt.Fprintln(stderr, err)
		return exitIncomplete
	}
	if !acknowledgeQuota {
		fmt.Fprintln(stderr, "probe execution refused: add --acknowledge-quota after reviewing the plan")
		return exitSafety
	}

	result := service.Execute(ctx, providerID, caseID, timeout)
	if err := probe.WriteResult(stdout, result, format); err != nil {
		fmt.Fprintln(stderr, err)
		return exitIncomplete
	}
	switch result.Status {
	case probe.StatusConfirmed:
		return exitOK
	case probe.StatusNotObserved:
		return exitFinding
	default:
		return exitIncomplete
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
	err = emitReport(stdout, result, options.format)
	if errors.Is(err, errUnsupportedFormat) {
		fmt.Fprintf(stderr, "unsupported format %q; use text, json, or sarif\n", options.format)
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

var errUnsupportedFormat = errors.New("unsupported report format")

func emitReport(writer io.Writer, value agentconfig.Report, format string) error {
	switch format {
	case "text":
		return report.WriteText(writer, value)
	case "json":
		return report.WriteJSON(writer, value)
	case "sarif":
		return report.WriteSARIF(writer, value)
	default:
		return errUnsupportedFormat
	}
}

func runPin(ctx context.Context, scanner *app.Scanner, args []string, stdout, stderr io.Writer) int {
	options, err := parseAnalysisOptions("pin", args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	if options.includeUserContext {
		fmt.Fprintln(stderr, "pin refuses --include-user-context because repository lockfiles must not encode user context")
		return exitSafety
	}
	result, scanErr := scanner.Scan(ctx, options.workspace, scanOptions(options))
	if scanErr != nil {
		return writeScanError(stderr, scanErr)
	}
	if !result.Complete {
		fmt.Fprintln(stderr, "refusing to pin an incomplete repository resolution")
		return exitIncomplete
	}
	if reachesThreshold(result.Findings, options.failOn) {
		if err := emitReport(stderr, result, options.format); err != nil {
			fmt.Fprintln(stderr, err)
		}
		return exitFinding
	}
	lock, buildErr := snapshot.Build(result)
	if buildErr != nil {
		fmt.Fprintln(stderr, buildErr)
		return exitIncomplete
	}
	if err := snapshot.WriteFile(options.workspace, options.output, lock); err != nil {
		if errors.Is(err, snapshot.ErrUnsafePath) {
			fmt.Fprintln(stderr, err)
			return exitSafety
		}
		fmt.Fprintln(stderr, err)
		return exitIncomplete
	}
	fmt.Fprintf(stdout, "Pinned repository snapshot: %s\nDigest: %s:%s\nEntries: %d\n",
		options.output, lock.LockDigest.Algorithm, lock.LockDigest.Value, len(lock.Entries))
	return exitOK
}

func runVerify(ctx context.Context, scanner *app.Scanner, args []string, stdout, stderr io.Writer) int {
	options, err := parseAnalysisOptions("verify", args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	if options.includeUserContext {
		fmt.Fprintln(stderr, "verify refuses --include-user-context because repository lockfiles must not encode user context")
		return exitSafety
	}
	if len(options.providers) > 0 || len(options.targets) > 0 {
		fmt.Fprintln(stderr, "verify uses providers and targets pinned in the snapshot; do not pass --provider or --target")
		return exitUsage
	}
	pinned, readErr := snapshot.ReadFile(options.workspace, options.snapshot, snapshot.MaxFileBytes)
	if readErr != nil {
		if errors.Is(readErr, snapshot.ErrUnsafePath) {
			fmt.Fprintln(stderr, readErr)
			return exitSafety
		}
		fmt.Fprintln(stderr, readErr)
		return exitUsage
	}
	result, scanErr := scanner.Scan(ctx, options.workspace, agentconfig.ScanOptions{
		Targets: pinned.Request.Targets, Providers: pinned.Request.Providers,
		FollowSymlinks: options.followSymlinks, MaxSourceBytes: options.maxSourceBytes, MaxImportDepth: options.maxImportDepth,
	})
	if scanErr != nil {
		return writeScanError(stderr, scanErr)
	}
	current, buildErr := snapshot.Build(result)
	if buildErr != nil {
		fmt.Fprintln(stderr, buildErr)
		return exitIncomplete
	}
	verified := snapshot.Equivalent(pinned, current)
	if !verified {
		deltas := snapshot.Diff(pinned, current)
		summary := fmt.Sprintf("Pinned repository snapshot differs: %d entry changes.", len(deltas))
		if len(deltas) == 0 {
			summary = "Pinned snapshot metadata differs from the current tool or adapter registry."
		}
		providers := make([]string, 0, len(deltas))
		targets := make([]string, 0, len(deltas))
		for _, delta := range deltas {
			providers = appendUnique(providers, delta.Provider)
			targets = appendUnique(targets, delta.Target)
		}
		result.Findings = append(result.Findings, agentconfig.Finding{
			Code: "ACI063", Severity: "error", Title: "Pinned repository snapshot drifted", Summary: summary,
			Providers: providers, Targets: targets, Confidence: "high",
			Remediation: []string{"Review repository instruction changes, then run pin intentionally to accept them."},
		})
	}
	if err := emitReport(stdout, result, options.format); err != nil {
		if errors.Is(err, errUnsupportedFormat) {
			fmt.Fprintf(stderr, "unsupported format %q; use text, json, or sarif\n", options.format)
			return exitUsage
		}
		fmt.Fprintln(stderr, err)
		return exitIncomplete
	}
	if options.format == "text" && verified {
		fmt.Fprintf(stdout, "Snapshot: verified (%s:%s)\n", pinned.LockDigest.Algorithm, pinned.LockDigest.Value)
	}
	if reachesThreshold(result.Findings, options.failOn) {
		return exitFinding
	}
	if !result.Complete {
		return exitIncomplete
	}
	return exitOK
}

func scanOptions(options commandOptions) agentconfig.ScanOptions {
	return agentconfig.ScanOptions{
		Targets: options.targets, Providers: options.providers,
		FollowSymlinks: options.followSymlinks, MaxSourceBytes: options.maxSourceBytes, MaxImportDepth: options.maxImportDepth,
	}
}

func writeScanError(stderr io.Writer, scanErr error) int {
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

func appendUnique(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func parseAnalysisOptions(command string, args []string, output io.Writer) (commandOptions, error) {
	options := commandOptions{
		workspace: ".", format: "text", maxSourceBytes: 1 << 20, maxImportDepth: 5, failOn: "error",
		output: "agent-config-inspector.lock.json", snapshot: "agent-config-inspector.lock.json",
	}
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
	flags.StringVar(&options.format, "format", options.format, "text, json, or sarif")
	flags.BoolVar(&options.includeUserContext, "include-user-context", false, "opt in to redacted user-level instructions")
	flags.BoolVar(&options.followSymlinks, "follow-workspace-symlinks", false, "follow symlinks that remain inside workspace")
	flags.Int64Var(&options.maxSourceBytes, "max-source-bytes", options.maxSourceBytes, "maximum bytes read from one source")
	flags.IntVar(&options.maxImportDepth, "max-import-depth", options.maxImportDepth, "maximum context import hops; provider caps apply")
	flags.StringVar(&options.failOn, "fail-on", options.failOn, "error, warning, or never")
	if command == "pin" {
		flags.StringVar(&options.output, "output", options.output, "workspace-relative repository lockfile path")
	}
	if command == "verify" {
		flags.StringVar(&options.snapshot, "snapshot", options.snapshot, "workspace-relative repository lockfile path")
	}
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() > 0 {
		return options, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	options.format = strings.ToLower(options.format)
	options.failOn = strings.ToLower(options.failOn)
	if options.format != "text" && options.format != "json" && options.format != "sarif" {
		return options, errors.New("--format must be text, json, or sarif")
	}
	if options.maxSourceBytes <= 0 {
		return options, errors.New("--max-source-bytes must be positive")
	}
	if options.maxImportDepth <= 0 || options.maxImportDepth > 5 {
		return options, errors.New("--max-import-depth must be between 1 and 5")
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
		"  agent-config-inspector pin [workspace] --output <file>",
		"  agent-config-inspector verify [workspace] --snapshot <file>",
		"  agent-config-inspector probe <provider> [--case <id>] [--execute --acknowledge-quota]",
		"  agent-config-inspector inventory skills [workspace] [--providers claude,codex] [--target <path>]",
		"  agent-config-inspector providers list",
		"  agent-config-inspector providers show <id>",
		"  agent-config-inspector version",
		"",
		"Supported provider aliases: claude, codex, copilot, gemini, kimi",
	}
	for _, line := range lines {
		fmt.Fprintln(writer, line)
	}
}

func writeInventoryUsage(writer io.Writer) {
	lines := []string{
		"Usage:",
		"  agent-config-inspector inventory skills [workspace] [options]",
		"",
		"Inventories repository-owned Agent Skills without showing descriptions or bodies.",
		"The selected target represents the provider launch or accessed path.",
		"",
		"Options:",
		"  --provider claude|codex",
		"  --providers claude,codex",
		"  --target <workspace-relative-path>",
		"  --format text|json",
		"  --fail-on error|warning|never",
		"  --follow-workspace-symlinks",
	}
	for _, line := range lines {
		fmt.Fprintln(writer, line)
	}
}

func writeProbeUsage(writer io.Writer) {
	lines := []string{
		"Usage:",
		"  agent-config-inspector probe <provider> [options]",
		"",
		"By default, probe prints a safe execution plan and makes no model request.",
		"Actual execution requires both --execute and --acknowledge-quota.",
		"",
		"Options:",
		"  --case root-instruction-discovery",
		"  --format text|json",
		"  --timeout 2m",
		"  --execute",
		"  --acknowledge-quota",
	}
	for _, line := range lines {
		fmt.Fprintln(writer, line)
	}
}
