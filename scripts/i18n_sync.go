package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// i18n_sync.go
// Small helper to ensure keys present in en.yaml are present in other locale YAMLs
// Usage:
//  go run ./scripts/i18n_sync.go -path=utils/locales           (dry-run)
//  go run ./scripts/i18n_sync.go -path=utils/locales -apply     (apply changes)
// Optional: -stub="NEEDS_TRANSLATION"

func main() {
	path := flag.String("path", "utils/locales", "path to locales directory")
	apply := flag.Bool("apply", false, "apply changes (append stubs) instead of dry-run")
	stub := flag.String("stub", "NEEDS_TRANSLATION", "stub value to insert for missing keys")
	verbose := flag.Bool("v", false, "verbose output")
	flag.Parse()

	baseFile := filepath.Join(*path, "en.yaml")
	baseKeys, err := loadKeys(baseFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading base en.yaml: %v\n", err)
		os.Exit(1)
	}

	files, err := ioutil.ReadDir(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading locales dir: %v\n", err)
		os.Exit(1)
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		if name == "en.yaml" {
			continue
		}
		target := filepath.Join(*path, name)
		tKeys, err := loadKeys(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v\n", target, err)
			continue
		}

		missing := []string{}
		for k := range baseKeys {
			if _, ok := tKeys[k]; !ok {
				missing = append(missing, k)
			}
		}
		sort.Strings(missing)

		if len(missing) == 0 {
			if *verbose {
				fmt.Printf("%s: OK (no missing keys)\n", name)
			}
			continue
		}

		fmt.Printf("%s: %d missing keys:\n", name, len(missing))
		for _, k := range missing {
			val := baseKeys[k]
			// Print a compact snippet showing the stub that would be inserted
			fmt.Printf("  - %s\n", k)
			fmt.Printf("      # English: %s\n", sanitizeForComment(val))
			fmt.Printf("      %s: \"%s\"\n", k, *stub)
		}

		if *apply {
			if err := backupFile(target); err != nil {
				fmt.Fprintf(os.Stderr, "failed to backup %s: %v\n", target, err)
				continue
			}
			if err := appendStubs(target, missing, baseKeys, *stub); err != nil {
				fmt.Fprintf(os.Stderr, "failed to append stubs to %s: %v\n", target, err)
				continue
			}
			fmt.Printf("  -> Applied: appended %d keys to %s\n", len(missing), name)
		}
	}
}

var keyRegex = regexp.MustCompile(`^\s*([a-z0-9_]+)\s*:\s*(.*)$`)

// loadKeys loads top-level keys from a simple YAML file. It does not fully parse YAML
// but extracts lines that look like `key: value` and returns value as the rest of the line.
func loadKeys(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		m := keyRegex.FindStringSubmatch(line)
		if m != nil {
			key := m[1]
			val := strings.TrimSpace(m[2])
			out[key] = val
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func sanitizeForComment(val string) string {
	// strip surrounding quotes if present
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) {
		val = val[1 : len(val)-1]
	}
	if strings.HasPrefix(val, `'`) && strings.HasSuffix(val, `'`) {
		val = val[1 : len(val)-1]
	}
	// condense whitespace
	val = strings.ReplaceAll(val, "\n", "\\n")
	val = strings.ReplaceAll(val, "\r", "")
	return val
}

func backupFile(path string) error {
	input, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	bak := fmt.Sprintf("%s.bak.%s", path, time.Now().Format("20060102T150405"))
	return ioutil.WriteFile(bak, input, 0644)
}

func appendStubs(path string, missing []string, base map[string]string, stub string) error {
	// Read existing content
	orig, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.Write(orig)
	if len(orig) > 0 && orig[len(orig)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString("\n# --- MISSING TRANSLATIONS ADDED BY i18n_sync.go ---\n")
	for _, k := range missing {
		eng := sanitizeForComment(base[k])
		buf.WriteString("# English: ")
		buf.WriteString(eng)
		buf.WriteString("\n")
		buf.WriteString(fmt.Sprintf("%s: \"%s\"\n\n", k, stub))
	}
	return ioutil.WriteFile(path, buf.Bytes(), 0644)
}
