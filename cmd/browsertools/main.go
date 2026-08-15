// Command browsertools provides offline, file-first profile authoring and
// review operations. It never launches a browser or contacts a network.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/adapter/crawl4ai"
	"github.com/OpenUdon/browsertools/adapter/firecrawl"
	"github.com/OpenUdon/browsertools/adapter/llmscraper"
	playwrightadapter "github.com/OpenUdon/browsertools/adapter/playwright"
	capabilitybundle "github.com/OpenUdon/browsertools/bundle"
	"github.com/OpenUdon/browsertools/cache"
	"github.com/OpenUdon/browsertools/draft"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/registry"
	"github.com/OpenUdon/browsertools/revalidate"
	"github.com/OpenUdon/browsertools/review"
	"gopkg.in/yaml.v3"
)

const (
	exitOK        = 0
	exitRejected  = 1
	exitUsageOrIO = 2
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		usage(stderr)
		return exitUsageOrIO
	}
	switch args[0] + " " + args[1] {
	case "profile validate":
		return runProfileValidate(args[2:], stdin, stdout, stderr)
	case "evidence import":
		return runEvidenceImport(args[2:], stdin, stdout, stderr)
	case "draft build":
		return runDraftBuild(args[2:], stdin, stdout, stderr)
	case "review bundle":
		return runReviewBundle(args[2:], stdin, stdout, stderr)
	case "revalidate check":
		return runRevalidate(args[2:], stdin, stdout, stderr)
	case "bundle build":
		return runCapabilityBundleBuild(args[2:], stdin, stdout, stderr)
	case "bundle verify":
		return runCapabilityBundleVerify(args[2:], stdin, stdout, stderr)
	case "registry publish":
		return runRegistryPublish(args[2:], stdin, stdout, stderr)
	case "registry search":
		return runRegistrySearch(args[2:], stdout, stderr)
	case "registry pull":
		return runRegistryPull(args[2:], stdout, stderr)
	case "registry verify":
		return runRegistryVerify(args[2:], stdout, stderr)
	case "cache put":
		return runCachePut(args[2:], stdin, stdout, stderr)
	case "cache get":
		return runCacheGet(args[2:], stdin, stdout, stderr)
	case "cache list":
		return runCacheList(args[2:], stdout, stderr)
	case "cache prune":
		return runCachePrune(args[2:], stdout, stderr)
	default:
		usage(stderr)
		return exitUsageOrIO
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: browsertools <profile validate|evidence import|draft build|review bundle|revalidate check|bundle build|bundle verify|registry publish|registry search|registry pull|registry verify|cache put|cache get|cache list|cache prune> [flags]")
}

func runRegistryPublish(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry publish", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit local static registry root")
	bundlePath := fs.String("bundle", "", "capability bundle JSON path or -")
	at := fs.String("at", "", "RFC3339 publication time")
	supersedes := fs.String("supersedes", "", "optional existing ID@RELEASE coordinate")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *bundlePath == "" || *at == "" {
		fmt.Fprintln(stderr, "registry publish: --root, --bundle, and --at are required")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "registry publish: invalid --at:", err)
		return exitUsageOrIO
	}
	data, err := readInput(*bundlePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	value, err := capabilitybundle.Parse(data)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	var prior *registry.Coordinate
	if *supersedes != "" {
		coordinate, parseErr := parseCoordinate(*supersedes)
		if parseErr != nil {
			fmt.Fprintln(stderr, "registry publish:", parseErr)
			return exitUsageOrIO
		}
		prior = &coordinate
	}
	report, err := registry.PublishLocal(context.Background(), registry.PublishOptions{
		Root: *root, Bundle: value, At: when, Supersedes: prior,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runRegistrySearch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	location := fs.String("location", "", "local registry root or static HTTPS base URL")
	query := fs.String("query", "", "id, title, origin, or action query")
	at := fs.String("at", "", "RFC3339 lifecycle evaluation time")
	limit := fs.Int("limit", registry.DefaultMaxResults, "maximum results")
	includeInactive := fs.Bool("include-inactive", false, "include stale, revoked, and superseded entries")
	format := fs.String("format", "json", "json or text")
	client, policy := registryClientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *location == "" || *at == "" {
		fmt.Fprintln(stderr, "registry search: --location and --at are required")
		return exitUsageOrIO
	}
	if err := setRegistryNetworkPolicy(client, *policy); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	client.MaxResults = *limit
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "registry search: invalid --at:", err)
		return exitUsageOrIO
	}
	report, err := client.Search(context.Background(), registry.SearchOptions{
		Location: *location, Query: *query, Limit: *limit, At: when, IncludeInactive: *includeInactive,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	switch *format {
	case "json":
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsageOrIO
		}
	case "text":
		for _, result := range report.Results {
			fmt.Fprintf(stdout, "%s@%s\t%s\t%d\t%s\t%s\n", result.Entry.ID, result.Entry.Release, result.Status, result.Score, result.Entry.Bundle.Digest.String(), result.Entry.Title)
		}
	default:
		fmt.Fprintln(stderr, "registry search: --format must be json or text")
		return exitUsageOrIO
	}
	return exitOK
}

func runRegistryPull(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry pull", flag.ContinueOnError)
	fs.SetOutput(stderr)
	location := fs.String("location", "", "local registry root or static HTTPS base URL")
	id := fs.String("id", "", "capability id")
	release := fs.String("release", "", "capability release")
	digestValue := fs.String("digest", "", "complete bundle sha256 digest")
	at := fs.String("at", "", "RFC3339 lifecycle evaluation time")
	allowInactive := fs.Bool("allow-inactive", false, "allow historical stale, revoked, or superseded content")
	out := fs.String("out", "-", "bundle output path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	client, policy := registryClientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	coordinateSelected := *id != "" || *release != ""
	if *location == "" || *at == "" || (coordinateSelected && (*id == "" || *release == "")) || (coordinateSelected == (*digestValue != "")) {
		fmt.Fprintln(stderr, "registry pull: --location, --at, and exactly one of (--id with --release) or --digest are required")
		return exitUsageOrIO
	}
	if err := setRegistryNetworkPolicy(client, *policy); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "registry pull: invalid --at:", err)
		return exitUsageOrIO
	}
	options := registry.PullOptions{Location: *location, Digest: *digestValue, At: when, AllowInactive: *allowInactive}
	if coordinateSelected {
		options.Coordinate = &registry.Coordinate{ID: *id, Release: *release}
	}
	result, err := client.Pull(context.Background(), options)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	data := append(append([]byte(nil), result.Content...), '\n')
	if len(result.Content) > 0 && result.Content[len(result.Content)-1] == '\n' {
		data = append([]byte(nil), result.Content...)
	}
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runRegistryVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	location := fs.String("location", "", "local registry root or static HTTPS base URL")
	at := fs.String("at", "", "RFC3339 lifecycle evaluation time")
	format := fs.String("format", "json", "json or text")
	client, policy := registryClientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *location == "" || *at == "" {
		fmt.Fprintln(stderr, "registry verify: --location and --at are required")
		return exitUsageOrIO
	}
	if err := setRegistryNetworkPolicy(client, *policy); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "registry verify: invalid --at:", err)
		return exitUsageOrIO
	}
	report, err := client.Verify(context.Background(), *location, when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	switch *format {
	case "json":
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsageOrIO
		}
	case "text":
		for _, entry := range report.Entries {
			fmt.Fprintf(stdout, "%s@%s\t%s\t%s\t%s\n", entry.Coordinate.ID, entry.Coordinate.Release, entry.Status, entry.Digest, entry.BlobPath)
		}
	default:
		fmt.Fprintln(stderr, "registry verify: --format must be json or text")
		return exitUsageOrIO
	}
	return exitOK
}

func runCapabilityBundleBuild(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bundle build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("id", "", "stable lowercase capability id")
	release := fs.String("release", "", "semantic release version")
	profilePath := fs.String("profile", "", "reviewed browser profile JSON/YAML path")
	reviewPath := fs.String("review", "", "promotable review bundle JSON path")
	evidencePath := fs.String("evidence", "", "normalized evidence JSON path")
	source := fs.String("source", "", "publication provenance source")
	license := fs.String("license", "", "SPDX-style license identifier")
	publishedAt := fs.String("published-at", "", "RFC3339 publication assessment time")
	out := fs.String("out", "-", "capability bundle JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	var authors stringList
	var companions stringList
	fs.Var(&authors, "author", "publication author; repeatable")
	fs.Var(&companions, "uws", "TARGET=PATH inert UWS companion; repeatable")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *id == "" || *release == "" || *profilePath == "" || *reviewPath == "" || *evidencePath == "" || *source == "" || *license == "" || *publishedAt == "" {
		fmt.Fprintln(stderr, "bundle build: --id, --release, --profile, --review, --evidence, --source, --license, and --published-at are required")
		return exitUsageOrIO
	}
	mappings, err := parsePathMappings(companions)
	if err != nil {
		fmt.Fprintln(stderr, "bundle build:", err)
		return exitUsageOrIO
	}
	stdinPaths := []string{*profilePath, *reviewPath, *evidencePath}
	for _, mapping := range mappings {
		stdinPaths = append(stdinPaths, mapping.source)
	}
	if stdinCount(stdinPaths...) > 1 {
		fmt.Fprintln(stderr, "bundle build: only one input may use stdin")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *publishedAt)
	if err != nil {
		fmt.Fprintln(stderr, "bundle build: invalid --published-at:", err)
		return exitUsageOrIO
	}
	prof, err := loadProfileInput(*profilePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return classifyProfileError(err)
	}
	reviewData, err := readInput(*reviewPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	var reviewed review.Bundle
	if err := decodeStrictJSON(reviewData, &reviewed); err != nil {
		fmt.Fprintln(stderr, "bundle build: decode review:", err)
		return exitUsageOrIO
	}
	records, err := readEvidenceStrict(*evidencePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	uwsCompanions := make([]capabilitybundle.Companion, 0, len(mappings))
	for _, mapping := range mappings {
		data, readErr := readInput(mapping.source, stdin)
		if readErr != nil {
			fmt.Fprintln(stderr, readErr)
			return exitUsageOrIO
		}
		mediaType, mediaErr := companionMediaType(mapping.target)
		if mediaErr != nil {
			fmt.Fprintln(stderr, mediaErr)
			return exitUsageOrIO
		}
		uwsCompanions = append(uwsCompanions, capabilitybundle.Companion{
			Path: mapping.target, MediaType: mediaType, Content: data,
		})
	}
	value, err := capabilitybundle.Build(capabilitybundle.BuildOptions{
		ID: *id, Release: *release, Source: *source, License: *license, Authors: authors,
		Profile: prof, Review: &reviewed, Evidence: records, Companions: uwsCompanions, PublishedAt: when,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	data, err := capabilitybundle.CanonicalJSON(value, when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runCapabilityBundleVerify(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bundle verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "capability bundle JSON path or -")
	at := fs.String("at", "", "RFC3339 verification time")
	format := fs.String("format", "text", "text or json")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *input == "" || *at == "" {
		fmt.Fprintln(stderr, "bundle verify: --input and --at are required")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "bundle verify: invalid --at:", err)
		return exitUsageOrIO
	}
	data, err := readInput(*input, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	value, err := capabilitybundle.Parse(data)
	if err == nil {
		err = capabilitybundle.Verify(value, when)
	}
	if err != nil {
		if *format == "json" {
			_ = json.NewEncoder(stdout).Encode(map[string]any{"valid": false, "errors": []string{err.Error()}})
		} else if *format == "text" {
			fmt.Fprintln(stderr, err)
		} else {
			fmt.Fprintln(stderr, "bundle verify: --format must be text or json")
			return exitUsageOrIO
		}
		return exitRejected
	}
	record, err := capabilitybundle.Digest(value, when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	switch *format {
	case "text":
		fmt.Fprintf(stdout, "valid\t%s\t%s\t%s\n", value.Payload.Identity.ID, value.Payload.Identity.Release, record.String())
	case "json":
		_ = json.NewEncoder(stdout).Encode(map[string]any{
			"valid": true, "id": value.Payload.Identity.ID, "release": value.Payload.Identity.Release, "digest": record.String(),
		})
	default:
		fmt.Fprintln(stderr, "bundle verify: --format must be text or json")
		return exitUsageOrIO
	}
	return exitOK
}

func runCachePut(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache put", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit private cache root")
	input := fs.String("input", "", "input path or -")
	kind := fs.String("kind", "", "private_raw, normalized_evidence, profile, or review_bundle")
	mediaType := fs.String("media-type", "", "artifact media type")
	createdAt := fs.String("created-at", "", "RFC3339 creation time")
	expiresAt := fs.String("expires-at", "", "optional RFC3339 expiry time")
	source := fs.String("source", "", "optional source tool or provenance label")
	publicationEligible := fs.Bool("publication-eligible", false, "mark a non-raw artifact eligible for independent publication verification")
	var annotations stringList
	fs.Var(&annotations, "annotation", "key=value annotation; repeatable")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *input == "" || *kind == "" || *mediaType == "" || *createdAt == "" {
		fmt.Fprintln(stderr, "cache put: --root, --input, --kind, --media-type, and --created-at are required")
		return exitUsageOrIO
	}
	created, err := time.Parse(time.RFC3339Nano, *createdAt)
	if err != nil {
		fmt.Fprintln(stderr, "cache put: invalid --created-at:", err)
		return exitUsageOrIO
	}
	var expires time.Time
	if *expiresAt != "" {
		expires, err = time.Parse(time.RFC3339Nano, *expiresAt)
		if err != nil {
			fmt.Fprintln(stderr, "cache put: invalid --expires-at:", err)
			return exitUsageOrIO
		}
	}
	annotationMap, err := parseKeyValues(annotations)
	if err != nil {
		fmt.Fprintln(stderr, "cache put:", err)
		return exitUsageOrIO
	}
	store, err := cache.Open(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	reader, closeInput, err := openInput(*input, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	defer closeInput()
	entry, err := store.Put(context.Background(), reader, cache.PutOptions{
		Kind: cache.Kind(*kind), MediaType: *mediaType, CreatedAt: created, ExpiresAt: expires,
		Source: *source, Annotations: annotationMap, PublicationEligible: *publicationEligible,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	if err := json.NewEncoder(stdout).Encode(entry); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runCacheGet(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = stdin
	fs := flag.NewFlagSet("cache get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit private cache root")
	id := fs.String("id", "", "sha256 cache id")
	at := fs.String("at", "", "RFC3339 expiry evaluation time")
	out := fs.String("out", "-", "payload output path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *id == "" || *at == "" {
		fmt.Fprintln(stderr, "cache get: --root, --id, and --at are required")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339Nano, *at)
	if err != nil {
		fmt.Fprintln(stderr, "cache get: invalid --at:", err)
		return exitUsageOrIO
	}
	store, err := cache.Open(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	_, payload, err := store.Get(context.Background(), *id, when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	if err := writeOutput(*out, payload, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runCacheList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit private cache root")
	at := fs.String("at", "", "RFC3339 expiry evaluation time")
	includeExpired := fs.Bool("include-expired", false, "include expired entries")
	format := fs.String("format", "json", "json or text")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *at == "" {
		fmt.Fprintln(stderr, "cache list: --root and --at are required")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339Nano, *at)
	if err != nil {
		fmt.Fprintln(stderr, "cache list: invalid --at:", err)
		return exitUsageOrIO
	}
	store, err := cache.Open(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	entries, err := store.List(context.Background(), when, *includeExpired)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	return renderCacheEntries(entries, *format, stdout, stderr)
}

func runCachePrune(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit private cache root")
	at := fs.String("at", "", "RFC3339 prune time")
	format := fs.String("format", "json", "json or text")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *at == "" {
		fmt.Fprintln(stderr, "cache prune: --root and --at are required")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339Nano, *at)
	if err != nil {
		fmt.Fprintln(stderr, "cache prune: invalid --at:", err)
		return exitUsageOrIO
	}
	store, err := cache.Open(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	entries, err := store.Prune(context.Background(), when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	return renderCacheEntries(entries, *format, stdout, stderr)
}

func renderCacheEntries(entries []cache.Entry, format string, stdout, stderr io.Writer) int {
	switch format {
	case "json":
		if err := json.NewEncoder(stdout).Encode(entries); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsageOrIO
		}
	case "text":
		for _, entry := range entries {
			fmt.Fprintf(stdout, "%s\t%s\t%d\t%s\t%t\n", entry.ID, entry.Kind, entry.SizeBytes, entry.ExpiresAt, entry.PublicationEligible)
		}
	default:
		fmt.Fprintln(stderr, "cache output format must be json or text")
		return exitUsageOrIO
	}
	return exitOK
}

func runProfileValidate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("profile validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "profile JSON/YAML path")
	format := fs.String("format", "text", "text or json")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *input == "" {
		fmt.Fprintln(stderr, "profile validate: --input is required")
		return exitUsageOrIO
	}
	_, err := loadProfileInput(*input, stdin)
	if err == nil {
		if *format == "json" {
			fmt.Fprintln(stdout, `{"valid":true,"errors":[]}`)
		} else if *format == "text" {
			fmt.Fprintln(stdout, "valid")
		} else {
			fmt.Fprintln(stderr, "profile validate: --format must be text or json")
			return exitUsageOrIO
		}
		return exitOK
	}
	var validationErr *profile.ValidationError
	if errors.As(err, &validationErr) {
		if *format == "json" {
			_ = json.NewEncoder(stdout).Encode(map[string]any{"valid": false, "errors": []string{err.Error()}})
		} else {
			fmt.Fprintln(stderr, err)
		}
		return exitRejected
	}
	fmt.Fprintln(stderr, err)
	return exitUsageOrIO
}

func runEvidenceImport(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("evidence import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tool := fs.String("adapter", "", "playwright, llm-scraper, crawl4ai, or firecrawl")
	input := fs.String("input", "", "raw fixture path or -")
	origin := fs.String("origin", "", "reviewed fixture origin")
	actionHint := fs.String("action-hint", "", "profile action name")
	redaction := fs.String("redaction-status", "", "not_required or redacted")
	out := fs.String("out", "-", "normalized evidence path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	var redactedFields stringList
	fs.Var(&redactedFields, "redacted-field", "redacted field path; repeatable")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *tool == "" || *input == "" || *origin == "" || *redaction == "" {
		fmt.Fprintln(stderr, "evidence import: --adapter, --input, --origin, and --redaction-status are required")
		return exitUsageOrIO
	}
	status := evidence.RedactionStatus(*redaction)
	if status != evidence.RedactionNotRequired && status != evidence.RedactionCompleted {
		fmt.Fprintln(stderr, "evidence import: --redaction-status must be not_required or redacted")
		return exitUsageOrIO
	}
	if status == evidence.RedactionCompleted && len(redactedFields) == 0 {
		fmt.Fprintln(stderr, "evidence import: redacted status requires at least one --redacted-field")
		return exitUsageOrIO
	}
	raw, err := readInput(*input, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	importer, err := importerFor(*tool)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	records, err := importer.Import(raw, adapter.Options{
		Origin: *origin, ActionHint: *actionHint, RedactionStatus: status, RedactedFields: redactedFields,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runDraftBuild(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("draft build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	evidencePath := fs.String("evidence", "", "normalized evidence JSON path or -")
	specPath := fs.String("spec", "", "draft spec JSON/YAML path")
	out := fs.String("out", "-", "profile path or -")
	format := fs.String("format", "json", "stdout format: json or yaml")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *evidencePath == "" || *specPath == "" {
		fmt.Fprintln(stderr, "draft build: --evidence and --spec are required")
		return exitUsageOrIO
	}
	if *evidencePath == "-" && *specPath == "-" {
		fmt.Fprintln(stderr, "draft build: only one input may use stdin")
		return exitUsageOrIO
	}
	records, err := readEvidence(*evidencePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	specData, err := readInput(*specPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	var spec draft.Spec
	if err := decodeJSONOrYAML(specData, filepath.Ext(*specPath), &spec); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	result, err := draft.Build(records, spec)
	if err != nil {
		fmt.Fprintln(stderr, err)
		if result != nil && len(result.Diagnostics) > 0 {
			_ = json.NewEncoder(stderr).Encode(result.Diagnostics)
		}
		return exitRejected
	}
	data, err := marshalProfile(result.Profile, *out, *format)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runReviewBundle(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("review bundle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profilePath := fs.String("profile", "", "profile path")
	evidencePath := fs.String("evidence", "", "normalized evidence JSON path or -")
	decisionPath := fs.String("decisions", "", "optional decision JSON/YAML path")
	at := fs.String("at", "", "RFC3339 assessment time")
	out := fs.String("out", "-", "bundle JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *profilePath == "" || *evidencePath == "" || *at == "" {
		fmt.Fprintln(stderr, "review bundle: --profile, --evidence, and --at are required")
		return exitUsageOrIO
	}
	if stdinCount(*profilePath, *evidencePath, *decisionPath) > 1 {
		fmt.Fprintln(stderr, "review bundle: only one input may use stdin")
		return exitUsageOrIO
	}
	prof, err := loadProfileInput(*profilePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return classifyProfileError(err)
	}
	records, err := readEvidence(*evidencePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	decisions, err := readDecisions(*decisionPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	now, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	bundle, err := review.Build(prof, records, decisions, now)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	data, _ := json.MarshalIndent(bundle, "", "  ")
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	if !bundle.Promotable() {
		return exitRejected
	}
	return exitOK
}

func runRevalidate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("revalidate check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profilePath := fs.String("profile", "", "profile path")
	evidencePath := fs.String("evidence", "", "normalized evidence JSON path or -")
	decisionPath := fs.String("decisions", "", "optional decision JSON/YAML path")
	at := fs.String("at", "", "RFC3339 assessment time")
	out := fs.String("out", "-", "report JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *profilePath == "" || *evidencePath == "" || *at == "" {
		fmt.Fprintln(stderr, "revalidate check: --profile, --evidence, and --at are required")
		return exitUsageOrIO
	}
	if stdinCount(*profilePath, *evidencePath, *decisionPath) > 1 {
		fmt.Fprintln(stderr, "revalidate check: only one input may use stdin")
		return exitUsageOrIO
	}
	prof, err := loadProfileInput(*profilePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return classifyProfileError(err)
	}
	records, err := readEvidence(*evidencePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	decisions, err := readDecisions(*decisionPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	now, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	result, err := revalidate.CheckAt(prof, records, decisions, now)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	if !result.OK {
		return exitRejected
	}
	return exitOK
}

func importerFor(name string) (adapter.Adapter, error) {
	switch name {
	case "playwright":
		return &playwrightadapter.Adapter{}, nil
	case "llm-scraper":
		return &llmscraper.Adapter{}, nil
	case "crawl4ai":
		return &crawl4ai.Adapter{}, nil
	case "firecrawl":
		return &firecrawl.Adapter{}, nil
	default:
		return nil, fmt.Errorf("unknown adapter %q", name)
	}
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}

func openInput(path string, stdin io.Reader) (io.Reader, func(), error) {
	if path == "-" {
		return stdin, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return file, func() { _ = file.Close() }, nil
}

func parseKeyValues(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("annotation %q must use non-empty key=value", value)
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("annotation key %q is duplicated", key)
		}
		result[key] = strings.TrimSpace(item)
	}
	return result, nil
}

func readEvidence(path string, stdin io.Reader) ([]evidence.Record, error) {
	data, err := readInput(path, stdin)
	if err != nil {
		return nil, err
	}
	var records []evidence.Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode evidence: %w", err)
	}
	return records, nil
}

func readEvidenceStrict(path string, stdin io.Reader) ([]evidence.Record, error) {
	data, err := readInput(path, stdin)
	if err != nil {
		return nil, err
	}
	var records []evidence.Record
	if err := decodeStrictJSON(data, &records); err != nil {
		return nil, fmt.Errorf("decode evidence: %w", err)
	}
	return records, nil
}

func readDecisions(path string, stdin io.Reader) ([]evidence.LocatorDecision, error) {
	if path == "" {
		return []evidence.LocatorDecision{}, nil
	}
	data, err := readInput(path, stdin)
	if err != nil {
		return nil, err
	}
	var decisions []evidence.LocatorDecision
	if err := decodeJSONOrYAML(data, filepath.Ext(path), &decisions); err != nil {
		return nil, fmt.Errorf("decode decisions: %w", err)
	}
	return decisions, nil
}

func loadProfileInput(path string, stdin io.Reader) (*profile.Profile, error) {
	if path != "-" {
		return profile.LoadFile(path)
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return profile.ParseJSON(data)
	}
	return profile.ParseYAML(data)
}

func stdinCount(paths ...string) int {
	count := 0
	for _, path := range paths {
		if path == "-" {
			count++
		}
	}
	return count
}

func decodeJSONOrYAML(data []byte, extension string, target any) error {
	extension = strings.ToLower(extension)
	if extension == ".json" || (extension == "" && bytes.HasPrefix(bytes.TrimSpace(data), []byte("{"))) || (extension == "" && bytes.HasPrefix(bytes.TrimSpace(data), []byte("["))) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		return decoder.Decode(target)
	}
	if extension == ".yaml" || extension == ".yml" || extension == "" {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		return decoder.Decode(target)
	}
	return fmt.Errorf("unsupported JSON/YAML extension %q", extension)
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not supported")
		}
		return err
	}
	return nil
}

func registryClientFlags(fs *flag.FlagSet) (*registry.Client, *string) {
	client := &registry.Client{}
	policy := fs.String("network", "never", "network policy: never, ask, or allow")
	fs.DurationVar(&client.Timeout, "timeout", registry.DefaultTimeout, "total registry read deadline (capped at 8s)")
	fs.Int64Var(&client.MaxBytes, "max-bytes", registry.DefaultMaxBytes, "per-file response bound (capped at 20 MiB)")
	fs.BoolVar(&client.AllowUnsafeHosts, "allow-unsafe-hosts", false, "allow localhost/private HTTPS hosts; intended only for local tests")
	return client, policy
}

func setRegistryNetworkPolicy(client *registry.Client, value string) error {
	switch registry.NetworkPolicy(strings.TrimSpace(value)) {
	case registry.NetworkNever, registry.NetworkAsk, registry.NetworkAllow:
		client.NetworkPolicy = registry.NetworkPolicy(strings.TrimSpace(value))
		return nil
	default:
		return fmt.Errorf("registry --network must be never, ask, or allow")
	}
}

func parseCoordinate(value string) (registry.Coordinate, error) {
	value = strings.TrimSpace(value)
	position := strings.LastIndex(value, "@")
	if position <= 0 || position == len(value)-1 {
		return registry.Coordinate{}, fmt.Errorf("coordinate %q must use ID@RELEASE", value)
	}
	return registry.Coordinate{ID: strings.TrimSpace(value[:position]), Release: strings.TrimSpace(value[position+1:])}, nil
}

type pathMapping struct {
	target string
	source string
}

func parsePathMappings(values []string) ([]pathMapping, error) {
	result := make([]pathMapping, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		target, source, ok := strings.Cut(value, "=")
		target, source = strings.TrimSpace(target), strings.TrimSpace(source)
		if !ok || target == "" || source == "" {
			return nil, fmt.Errorf("UWS companion %q must use TARGET=PATH", value)
		}
		if _, ok := seen[target]; ok {
			return nil, fmt.Errorf("UWS companion target %q is duplicated", target)
		}
		seen[target] = struct{}{}
		result = append(result, pathMapping{target: target, source: source})
	}
	return result, nil
}

func companionMediaType(target string) (string, error) {
	lower := strings.ToLower(target)
	switch {
	case strings.HasSuffix(lower, ".uws.json"):
		return capabilitybundle.UWSJSONMediaType, nil
	case strings.HasSuffix(lower, ".uws.yaml"), strings.HasSuffix(lower, ".uws.yml"):
		return capabilitybundle.UWSYAMLMediaType, nil
	default:
		return "", fmt.Errorf("UWS companion target %q must end in .uws.json, .uws.yaml, or .uws.yml", target)
	}
}

func marshalProfile(prof *profile.Profile, outputPath, stdoutFormat string) ([]byte, error) {
	format := stdoutFormat
	if outputPath != "-" {
		switch strings.ToLower(filepath.Ext(outputPath)) {
		case ".json":
			format = "json"
		case ".yaml", ".yml":
			format = "yaml"
		default:
			return nil, fmt.Errorf("profile output must use .json, .yaml, or .yml")
		}
	}
	switch format {
	case "json":
		data, err := json.MarshalIndent(prof, "", "  ")
		return append(data, '\n'), err
	case "yaml":
		return profile.MarshalYAML(*prof)
	default:
		return nil, fmt.Errorf("draft build: --format must be json or yaml")
	}
}

func writeOutput(path string, data []byte, force bool, stdout io.Writer) error {
	if path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("refusing to overwrite %s without --force", path)
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func classifyProfileError(err error) int {
	var validationErr *profile.ValidationError
	if errors.As(err, &validationErr) {
		return exitRejected
	}
	return exitUsageOrIO
}

type stringList []string

func (s *stringList) String() string         { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error { *s = append(*s, value); return nil }
