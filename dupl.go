package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	// Flags for aggressive normalization
	normalizeScheme     = flag.Bool("normalize-scheme", false, "Treat http and https as same (use https)")
	removeWWW           = flag.Bool("remove-www", false, "Remove 'www.' subdomain from host")
	removeTrailingSlash = flag.Bool("remove-trailing-slash", false, "Remove trailing slash from path")
	ignoreQueryVals     = flag.Bool("ignore-query-vals", false, "Treat all query values as {val} (ignore actual values)")
	lowercasePath       = flag.Bool("lowercase-path", false, "Convert path to lowercase (case-insensitive dedup)")
	keepFragment        = flag.Bool("keep-fragment", false, "Keep URL fragment (#anchor) in dedup key (default false)")
)

var numRegex = regexp.MustCompile(`^\d+$`)

func normalizePath(path string, lowercase bool) string {
	// Remove duplicate slashes
	path = strings.ReplaceAll(path, "//", "/")

	// Remove trailing slash if requested
	if *removeTrailingSlash && len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}

	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		if numRegex.MatchString(seg) {
			segments[i] = "{id}"
		}
		if *lowercasePath {
			segments[i] = strings.ToLower(segments[i])
		}
	}
	return strings.Join(segments, "/")
}

func normalizeHost(host string) string {
	host = strings.ToLower(host)
	if *removeWWW {
		host = strings.TrimPrefix(host, "www.")
		host = strings.TrimPrefix(host, "www2.") // some sites use www2
	}
	// Remove default ports? Optional, but can cause issues.
	// Leave as-is for now.
	return host
}

func normalize(u *url.URL) string {
	// Normalize scheme
	scheme := u.Scheme
	if *normalizeScheme {
		if scheme == "http" || scheme == "https" {
			scheme = "https" // or could be "http", but https is modern default
		}
	}

	// Normalize host
	host := normalizeHost(u.Host)

	// Normalize path
	path := normalizePath(u.Path, *lowercasePath)

	// For root path, avoid "//" (scheme://host//path? I fixed duplicate slashes above)
	if path == "" {
		path = "/"
	}

	// Build key without fragment initially
	key := scheme + "://" + host + path

	// Process query parameters
	params := u.Query()
	if len(params) > 0 {
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			vals := params[k]
			normalizedVals := make([]string, 0, len(vals))
			for _, v := range vals {
				if *ignoreQueryVals {
					normalizedVals = append(normalizedVals, "{val}")
				} else if numRegex.MatchString(v) {
					normalizedVals = append(normalizedVals, "{id}")
				} else {
					normalizedVals = append(normalizedVals, "{val}")
				}
			}
			sort.Strings(normalizedVals)
			parts = append(parts, k+"="+strings.Join(normalizedVals, ","))
		}
		key += "?" + strings.Join(parts, "&")
	}

	// Add fragment if needed (usually ignored for dedup, but optional)
	if *keepFragment && u.Fragment != "" {
		key += "#" + u.Fragment
	}

	return key
}

func main() {
	flag.Parse()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	seen := make(map[string]string)
	var order []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Handle scheme-relative URLs (//example.com/path) by prepending a dummy scheme
		if strings.HasPrefix(line, "//") {
			line = "http:" + line
		}

		u, err := url.Parse(line)
		if err != nil {
			continue
		}

		if u.Host == "" {
			continue
		}

		key := normalize(u)
		if _, exists := seen[key]; !exists {
			seen[key] = line
			order = append(order, line)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "error reading input:", err)
		os.Exit(1)
	}

	for _, u := range order {
		fmt.Println(u)
	}
}
