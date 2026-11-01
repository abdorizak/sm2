package process

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// loadEnvFiles reads .env files in order and returns their KEY=VALUE pairs;
// a later file overrides an earlier one. A missing or malformed file is an
// error, so a typo in a path can't silently start an app without its config.
func loadEnvFiles(paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("env file: %w", err)
		}
		pairs, err := parseDotenv(data)
		if err != nil {
			return nil, fmt.Errorf("env file %s: %w", p, err)
		}
		out = append(out, pairs...)
	}
	return out, nil
}

// parseDotenv parses the common .env format:
//
//	# comment
//	KEY=value
//	export KEY=value
//	KEY="double quoted, with \n escapes"
//	KEY='single quoted, literal'
//	KEY=value # trailing comment (unquoted values only)
//
// Values are taken literally: no ${VAR} expansion.
func parseDotenv(data []byte) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if n == 1 {
			line = strings.TrimPrefix(line, "\ufeff") // UTF-8 BOM
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("line %d: want KEY=VALUE", n)
		}
		val = strings.TrimSpace(val)
		switch {
		case strings.HasPrefix(val, `"`):
			end := closingQuote(val)
			if end < 0 {
				return nil, fmt.Errorf("line %d: unterminated quote", n)
			}
			s, err := strconv.Unquote(val[:end+1])
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n, err)
			}
			val = s
		case strings.HasPrefix(val, "'"):
			end := strings.IndexByte(val[1:], '\'')
			if end < 0 {
				return nil, fmt.Errorf("line %d: unterminated quote", n)
			}
			val = val[1 : end+1]
		default:
			if i := strings.Index(val, " #"); i >= 0 {
				val = strings.TrimSpace(val[:i])
			}
		}
		out = append(out, key+"="+val)
	}
	return out, sc.Err()
}

// closingQuote finds the index of the double quote that closes s[0],
// skipping backslash-escaped quotes.
func closingQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

// envKeys returns just the variable names from KEY=VALUE pairs (for display
// without leaking secrets).
func envKeys(pairs []string) []string {
	keys := make([]string, 0, len(pairs))
	seen := map[string]bool{}
	for _, p := range pairs {
		k, _, _ := strings.Cut(p, "=")
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

// EnvFileKeys lists the variable names an app's env files define, for
// `sm2 describe`. Unreadable files are reported in err.
func EnvFileKeys(paths []string) ([]string, error) {
	pairs, err := loadEnvFiles(paths)
	return envKeys(pairs), err
}

// filterEnv copies base, dropping inherited variables whose names start with
// any of prefixes ("*" drops them all), like pm2's filter_env.
func filterEnv(base, prefixes []string) []string {
	out := make([]string, 0, len(base))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, p := range prefixes {
			if p == "*" || strings.HasPrefix(k, p) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}
