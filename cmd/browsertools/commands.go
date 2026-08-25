package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/OpenUdon/browsertools/cache"
)

type commandIO struct {
	stdin          io.Reader
	stdout, stderr io.Writer
}

type commandHandler func([]string, commandIO) int

type commandSpec struct {
	group, name, summary string
	run                  commandHandler
}

func withInput(run func([]string, io.Reader, io.Writer, io.Writer) int) commandHandler {
	return func(args []string, streams commandIO) int {
		return run(args, streams.stdin, streams.stdout, streams.stderr)
	}
}

func withoutInput(run func([]string, io.Writer, io.Writer) int) commandHandler {
	return func(args []string, streams commandIO) int {
		return run(args, streams.stdout, streams.stderr)
	}
}

var commandRegistry = []commandSpec{
	{group: "profile", name: "validate", summary: "validate a browser profile", run: withInput(runProfileValidate)},
	{group: "auth-profile", name: "validate", summary: "validate an authentication profile", run: withInput(runAuthProfileValidate)},
	{group: "auth-draft", name: "build", summary: "build an authentication profile draft", run: withInput(runAuthDraftBuild)},
	{group: "auth-review", name: "bundle", summary: "build an authentication review bundle", run: withInput(runAuthReviewBundle)},
	{group: "registration-profile", name: "validate", summary: "validate a registration profile", run: withInput(runRegistrationProfileValidate)},
	{group: "registration-draft", name: "build", summary: "build a registration profile draft", run: withInput(runRegistrationDraftBuild)},
	{group: "registration-review", name: "bundle", summary: "build a registration review bundle", run: withInput(runRegistrationReviewBundle)},
	{group: "auth-assist", name: "chromium", summary: "observe explicit headed authentication flows", run: withInput(runAuthAssistChromium)},
	{group: "author-session", name: "chromium", summary: "serve a bounded headed authoring session", run: withInput(runAuthorSessionChromium)},
	{group: "registration-author-session", name: "chromium", summary: "serve bounded no-submit registration authoring", run: withInput(runRegistrationAuthorSessionChromium)},
	{group: "evidence", name: "import", summary: "import reviewed adapter evidence", run: withInput(runEvidenceImport)},
	{group: "draft", name: "build", summary: "build a browser profile draft", run: withInput(runDraftBuild)},
	{group: "review", name: "bundle", summary: "build a profile review bundle", run: withInput(runReviewBundle)},
	{group: "revalidate", name: "check", summary: "revalidate a profile against evidence", run: withInput(runRevalidate)},
	{group: "bundle", name: "build", summary: "build a capability bundle", run: withInput(runCapabilityBundleBuild)},
	{group: "bundle", name: "verify", summary: "verify a capability bundle", run: withInput(runCapabilityBundleVerify)},
	{group: "registry", name: "publish", summary: "publish to a local static registry", run: withInput(runRegistryPublish)},
	{group: "registry", name: "search", summary: "search a local or HTTPS registry", run: withoutInput(runRegistrySearch)},
	{group: "registry", name: "pull", summary: "pull an exact registry bundle", run: withoutInput(runRegistryPull)},
	{group: "registry", name: "verify", summary: "verify a local or HTTPS registry", run: withoutInput(runRegistryVerify)},
	{group: "cache", name: "put", summary: "store a bounded private artifact", run: withInput(runCachePut)},
	{group: "cache", name: "get", summary: "read an unexpired cache artifact", run: withInput(runCacheGet)},
	{group: "cache", name: "list", summary: "list cache manifests", run: withoutInput(runCacheList)},
	{group: "cache", name: "prune", summary: "prune expired cache artifacts", run: withoutInput(runCachePrune)},
	{group: "cache", name: "delete", summary: "delete an exact cache artifact", run: withoutInput(runCacheDelete)},
	{group: "capture", name: "chromium", summary: "perform explicit bounded headless capture", run: withoutInput(runCaptureChromium)},
	{group: "rich-capture", name: "chromium", summary: "perform bounded rich headless capture", run: withoutInput(runRichCaptureChromium)},
	{group: "guide", name: "author", summary: "run guided profile authoring", run: withInput(runGuideAuthor)},
	{group: "live-check", name: "chromium", summary: "observe declared current-page shapes", run: withInput(runLiveCheckChromium)},
	{group: "portability", name: "check", summary: "compare profile probes across engines", run: withInput(runPortabilityCheck)},
	{group: "playwright", name: "doctor", summary: "inspect installed Playwright support", run: withoutInput(runPlaywrightDoctor)},
	{group: "playwright", name: "capabilities", summary: "print supported Playwright capabilities", run: withoutInput(runPlaywrightCapabilities)},
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	streams := commandIO{stdin: stdin, stdout: stdout, stderr: stderr}
	if len(args) == 1 && isHelp(args[0]) {
		printTopHelp(stdout)
		return exitOK
	}
	if len(args) == 2 && args[0] == "help" {
		if printGroupHelp(stdout, args[1]) {
			return exitOK
		}
		fmt.Fprintf(stderr, "browsertools: unknown command group %q\n", args[1])
		return exitUsageOrIO
	}
	if len(args) == 2 && isHelp(args[1]) {
		if printGroupHelp(stdout, args[0]) {
			return exitOK
		}
	}
	if len(args) < 2 {
		printTopHelp(stderr)
		return exitUsageOrIO
	}
	spec, ok := findCommand(args[0], args[1])
	if !ok {
		fmt.Fprintf(stderr, "browsertools: unknown command %q\n", strings.Join(args[:2], " "))
		printGroupHelp(stderr, args[0])
		return exitUsageOrIO
	}
	if len(args) == 3 && isHelp(args[2]) {
		helpStreams := streams
		helpStreams.stderr = stdout
		_ = spec.run([]string{"--help"}, helpStreams)
		return exitOK
	}
	return spec.run(args[2:], streams)
}

func findCommand(group, name string) (commandSpec, bool) {
	for _, spec := range commandRegistry {
		if spec.group == group && spec.name == name {
			return spec, true
		}
	}
	return commandSpec{}, false
}

func printTopHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: browsertools <group> <command> [flags]")
	fmt.Fprintln(w, "       browsertools help <group>")
	fmt.Fprintln(w, "command groups:")
	groups := make([]string, 0)
	for _, spec := range commandRegistry {
		if !slices.Contains(groups, spec.group) {
			groups = append(groups, spec.group)
		}
	}
	slices.Sort(groups)
	for _, group := range groups {
		fmt.Fprintf(w, "  %s\n", group)
	}
}

func printGroupHelp(w io.Writer, group string) bool {
	commands := make([]commandSpec, 0)
	for _, spec := range commandRegistry {
		if spec.group == group {
			commands = append(commands, spec)
		}
	}
	if len(commands) == 0 {
		return false
	}
	slices.SortFunc(commands, func(a, b commandSpec) int { return strings.Compare(a.name, b.name) })
	fmt.Fprintf(w, "usage: browsertools %s <command> [flags]\n", group)
	for _, spec := range commands {
		fmt.Fprintf(w, "  %-14s %s\n", spec.name, spec.summary)
	}
	return true
}

func isHelp(value string) bool {
	return value == "help" || value == "-h" || value == "--help"
}

func validFormat(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}

func validCacheKind(value cache.Kind) bool {
	return slices.Contains([]cache.Kind{
		cache.KindPrivateRaw, cache.KindNormalizedEvidence, cache.KindProfile, cache.KindReviewBundle,
	}, value)
}

func validSHA256ID(value string) bool {
	algorithm, encoded, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok || algorithm != "sha256" || len(encoded) != 64 || encoded != strings.ToLower(encoded) {
		return false
	}
	decoded, err := hex.DecodeString(encoded)
	return err == nil && len(decoded) == 32
}
