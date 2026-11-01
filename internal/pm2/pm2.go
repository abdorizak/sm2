// Package pm2 converts PM2 ecosystem files (ecosystem.config.js, .cjs, .mjs,
// .json, .yml) into an sm2.yaml, so a team can move from pm2 without
// rewriting its process definitions. Anything that doesn't carry over is
// reported as a note, and written as a comment above the app it concerns.
package pm2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Result is a converted ecosystem file.
type Result struct {
	YAML  []byte
	Apps  []string // sm2 app names, in file order
	Notes []string // "app: note" lines (global notes have no prefix)
}

// Convert reads the ecosystem file at path and renders an sm2.yaml for it.
// outDir is where the sm2.yaml will live (relative paths are written relative
// to it). envName selects pm2's env_<name> block, like `pm2 start --env name`.
func Convert(path, outDir, envName string) (*Result, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	raw, err := readEcosystem(abs)
	if err != nil {
		return nil, err
	}
	apps, global := appsOf(raw)
	if len(apps) == 0 {
		return nil, fmt.Errorf("%s: no apps found (expected module.exports = { apps: [...] })", path)
	}
	c := &converter{baseDir: filepath.Dir(abs), outDir: outDir, envName: envName, names: map[string]bool{}}

	root := mapping()
	appsNode := mapping()
	res := &Result{}
	for _, a := range apps {
		name, node, notes := c.app(a)
		key := scalar(name)
		if len(notes) > 0 {
			key.HeadComment = "pm2: " + strings.Join(notes, "\npm2: ")
		}
		appsNode.Content = append(appsNode.Content, key, node)
		res.Apps = append(res.Apps, name)
		for _, n := range notes {
			res.Notes = append(res.Notes, name+": "+n)
		}
	}
	res.Notes = append(global, res.Notes...)

	appsKey := scalar("apps")
	root.Content = append(root.Content, appsKey, appsNode)
	head := []string{
		"Converted from " + filepath.Base(abs) + " by `sm2 import`.",
	}
	if envName != "" {
		head = append(head, "Environment: env + env_"+envName+" (like `pm2 start --env "+envName+"`).")
	}
	for _, g := range global {
		head = append(head, "pm2: "+g)
	}
	head = append(head,
		"Review any \"pm2:\" notes below, then:",
		"  sm2 config validate -c sm2.yaml",
		"  sm2 config reload -c sm2.yaml")
	root.HeadComment = strings.Join(head, "\n")

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}); err != nil {
		return nil, err
	}
	enc.Close()
	res.YAML = buf.Bytes()
	return res, nil
}

// ---- reading ----

// evalScript loads an ecosystem module (CommonJS or ESM) and prints it as JSON.
// Functions are dropped; values computed at load time (process.env, path
// joins, …) come out resolved.
const evalScript = `
import { pathToFileURL } from "node:url";
const m = await import(pathToFileURL(process.argv[1]).href);
const conf = m.default ?? m;
process.stdout.write(JSON.stringify(conf, (k, v) => (typeof v === "function" ? undefined : v)));
`

func readEcosystem(path string) (any, error) {
	var data []byte
	var err error
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		data, err = os.ReadFile(path)
	case ".yml", ".yaml":
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var v any
		if err := yaml.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		return v, nil
	default: // .js, .cjs, .mjs: it's code, so let Node evaluate it
		data, err = evalJS(path)
	}
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return v, nil
}

func evalJS(path string) ([]byte, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, fmt.Errorf("%s is JavaScript, so reading it needs Node.js (not found on PATH).\n"+
			"Install node, or export it to JSON where node is available and import that:\n"+
			"  node -e 'console.log(JSON.stringify(require(\"./%s\")))' > ecosystem.json",
			filepath.Base(path), filepath.Base(path))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "--input-type=module", "-e", evalScript, path)
	cmd.Dir = filepath.Dir(path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("node could not load %s:\n%s", filepath.Base(path), msg)
	}
	return out, nil
}

// appsOf finds the app list in an ecosystem document: {apps: [...]},
// {apps: {...}}, a bare list, or a single app object.
func appsOf(v any) (apps []map[string]any, global []string) {
	add := func(x any) {
		if m, ok := x.(map[string]any); ok {
			apps = append(apps, m)
		}
	}
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			add(x)
		}
	case map[string]any:
		if d, ok := t["deploy"]; ok && d != nil {
			global = append(global, "the deploy section isn't converted (sm2 has no deploy command); keep your existing deploy step")
		}
		switch a := t["apps"].(type) {
		case []any:
			for _, x := range a {
				add(x)
			}
		case map[string]any:
			add(a)
		default:
			if _, ok := t["script"]; ok {
				add(t)
			}
		}
	}
	return apps, global
}

// ---- converting ----

type converter struct {
	baseDir string // directory of the ecosystem file (pm2's default cwd)
	outDir  string
	envName string
	names   map[string]bool
}

// ignored are pm2 options with no sm2 equivalent, and why that's (mostly) fine.
var ignored = map[string]string{
	"out_file":              "logs go to ~/.sm2/logs/<name>.stdout.log / .stderr.log",
	"error_file":            "logs go to ~/.sm2/logs/<name>.stdout.log / .stderr.log",
	"log_file":              "logs go to ~/.sm2/logs/<name>.stdout.log / .stderr.log",
	"log":                   "logs go to ~/.sm2/logs/<name>.stdout.log / .stderr.log",
	"output":                "logs go to ~/.sm2/logs/<name>.stdout.log / .stderr.log",
	"error":                 "logs go to ~/.sm2/logs/<name>.stdout.log / .stderr.log",
	"pid_file":              "sm2 tracks PIDs itself",
	"log_date_format":       "log timestamps aren't supported yet",
	"time":                  "log timestamps aren't supported yet",
	"merge_logs":            "each instance logs to its own file",
	"combine_logs":          "each instance logs to its own file",
	"log_type":              "logs are plain text",
	"disable_logs":          "logs are always written (rotate them with sm2 set logs.*)",
	"listen_timeout":        "sm2 doesn't wait for a ready signal",
	"wait_ready":            "sm2 doesn't wait for process.send('ready')",
	"shutdown_with_message": "sm2 stops apps with SIGTERM, then SIGKILL after kill_timeout",
	"kill_retry_time":       "sm2 stops apps with SIGTERM, then SIGKILL after kill_timeout",
	"treekill":              "sm2 always signals the whole process group",
	"stop_exit_codes":       "sm2 restarts on any exit under policy: always; use policy: on-failure if exit 0 means done",
	"user":                  "apps run as the user running the sm2 agent",
	"uid":                   "apps run as the user running the sm2 agent",
	"gid":                   "apps run as the user running the sm2 agent",
	"watch_options":         "sm2 polls for changes; chokidar options don't apply",
	"watch_delay":           "sm2 polls for changes once a second",
	"increment_var":         "not supported; set the variable per app instead",
	"source_map_support":    "not applicable",
	"vizion":                "not applicable",
	"post_update":           "not applicable",
	"force":                 "not applicable",
	"automation":            "not applicable",
	"pmx":                   "not applicable",
	"disable_trace":         "not applicable",
	"trace":                 "not applicable",
	"version":               "not applicable",
}

var safeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (c *converter) app(a map[string]any) (string, *yaml.Node, []string) {
	var notes []string
	note := func(f string, args ...any) { notes = append(notes, fmt.Sprintf(f, args...)) }
	out := mapping()
	set := func(k string, v *yaml.Node) { out.Content = append(out.Content, scalar(k), v) }

	script, _ := str(a["script"])

	// Name: required by sm2, and used in log file names, so keep it path-safe.
	name, _ := str(a["name"])
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(script), filepath.Ext(script))
		if name == "" || name == "." {
			name = "app"
		}
		note("no name in pm2; named %q after the script", name)
	}
	if ok, _ := boolean(a["append_env_to_name"]); ok && c.envName != "" {
		name += "-" + c.envName
	}
	if clean := strings.Trim(safeName.ReplaceAllString(name, "-"), "-"); clean != name {
		note("renamed %q to %q (sm2 names are used in log file paths)", name, clean)
		name = clean
	}
	for base, i := name, 2; c.names[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	c.names[name] = true

	// Working directory: pm2 defaults to where the ecosystem file is.
	cwd, _ := str(a["cwd"])
	absCwd := c.baseDir
	if cwd != "" {
		absCwd = cwd
		if !filepath.IsAbs(cwd) {
			absCwd = filepath.Join(c.baseDir, cwd)
		}
	}

	command, cmdNotes := c.command(a, script, absCwd)
	notes = append(notes, cmdNotes...)
	if command == "" {
		note("no script; set command by hand")
	}
	set("command", scalar(command))
	set("directory", scalar(c.rel(absCwd)))

	if ns, _ := str(a["namespace"]); ns != "" {
		set("namespace", scalar(ns))
	}

	// Instances. pm2's cluster mode shares one port between instances via
	// Node's cluster module; sm2 runs independent processes.
	mode, _ := str(a["exec_mode"])
	cluster := strings.HasPrefix(mode, "cluster")
	if v, ok := a["instances"]; ok {
		n, isNum := num(v)
		s, _ := str(v)
		cpus := runtime.NumCPU()
		switch {
		case s == "max" || (isNum && n == 0):
			note("instances %q resolved to %d (this machine's CPU count); adjust for the server", s, cpus)
			n = float64(cpus)
		case isNum && n < 0:
			note("instances %v resolved to %d (CPUs %v on this machine); adjust for the server", n, cpus+int(n), n)
			n = float64(cpus + int(n))
		}
		if n > 1 {
			set("instances", intNode(int(n)))
			cluster = cluster || script != ""
		}
	}
	if cluster {
		note("cluster mode: sm2 runs each instance as its own process, so they can't share one port. " +
			"Give each instance its own port (NODE_APP_INSTANCE is 0, 1, …) or put a load balancer in front, or use instances: 1")
	}

	// Restart behavior. pm2 restarts on any exit, even a clean one.
	restart := mapping()
	if ar, ok := boolean(a["autorestart"]); ok && !ar {
		restart.Content = append(restart.Content, scalar("policy"), scalar("never"))
	} else {
		restart.Content = append(restart.Content, scalar("policy"), scalar("always"))
	}
	// pm2's max_restarts counts consecutive *unstable* restarts (runs shorter
	// than min_uptime, default 1s); restart.min_uptime gives sm2 the same rule.
	if n, ok := num(a["max_restarts"]); ok && n > 0 {
		restart.Content = append(restart.Content, scalar("max_retries"), intNode(int(n)))
		mu := "1s"
		if v, ok := a["min_uptime"]; ok {
			mu = pm2Duration(v)
		}
		restart.Content = append(restart.Content, scalar("min_uptime"), scalar(mu))
	} else if _, ok := a["min_uptime"]; ok {
		restart.Content = append(restart.Content, scalar("min_uptime"), scalar(pm2Duration(a["min_uptime"])))
	}
	set("restart", restart)

	for _, k := range []string{"kill_timeout", "restart_delay", "exp_backoff_restart_delay"} {
		if n, ok := num(a[k]); ok && n > 0 {
			set(k, scalar(msDuration(n)))
		}
	}
	if m, ok := str(a["max_memory_restart"]); ok && m != "" {
		set("max_memory_restart", scalar(m))
	}

	// Watching.
	switch w := a["watch"].(type) {
	case bool:
		if w {
			set("watch", boolNode(true))
		}
	case string, []any:
		set("watch", boolNode(true))
		var paths []string
		for _, p := range list(w) {
			if !filepath.IsAbs(p) {
				p = filepath.Join(absCwd, p)
			}
			paths = append(paths, c.rel(p))
		}
		set("watch_paths", seq(paths))
	}
	if ig := list(a["ignore_watch"]); len(ig) > 0 {
		set("ignore_watch", seq(ig))
		for _, p := range ig {
			if strings.ContainsAny(p, `[]\^$*+?(){}|`) {
				note("ignore_watch %q is a pattern; sm2 matches plain path fragments (and always skips .git, node_modules). Check it", p)
			}
		}
	}
	if v := a["filter_env"]; v != nil {
		switch t := v.(type) {
		case bool:
			if t {
				set("filter_env", boolNode(true))
			}
		default:
			if l := list(t); len(l) > 0 {
				set("filter_env", seq(l))
			}
		}
	}
	if iv, _ := str(a["instance_var"]); iv != "" && iv != "NODE_APP_INSTANCE" {
		set("instance_var", scalar(iv))
	}

	if cr, ok := str(a["cron_restart"]); ok && cr != "" {
		f := strings.Fields(cr)
		switch {
		case len(f) == 6 && f[0] == "0":
			cr = strings.Join(f[1:], " ")
			note("cron_restart: dropped the leading seconds field (sm2 cron is 5-field)")
		case len(f) == 6:
			note("cron_restart %q has a seconds field sm2 doesn't support; fix it by hand", cr)
		}
		set("cron_restart", scalar(cr))
	}

	if as, ok := boolean(a["autostart"]); ok && !as {
		set("no_autostart", boolNode(true))
	}

	// Environment: env, then env_<name> on top (pm2 start --env <name>).
	env := map[string]string{}
	addEnv := func(v any, from string) {
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		for k, val := range m {
			if s, ok := str(val); ok {
				env[k] = s
				continue
			}
			b, _ := json.Marshal(val)
			env[k] = string(b)
			note("%s.%s is not a plain value; stored as JSON text", from, k)
		}
	}
	addEnv(a["env"], "env")
	var otherEnvs []string
	for k := range a {
		if strings.HasPrefix(k, "env_") && k != "env_"+c.envName && k != "env_file" {
			otherEnvs = append(otherEnvs, k)
		}
	}
	if c.envName != "" {
		key := "env_" + c.envName
		if v, ok := a[key]; ok {
			addEnv(v, key)
		} else if len(otherEnvs) > 0 {
			note("no %s block (has %s); used env only", key, strings.Join(otherEnvs, ", "))
			otherEnvs = nil
		}
	}
	if len(otherEnvs) > 0 {
		sort.Strings(otherEnvs)
		note("%s not applied (choose one with: sm2 import --env <name>)", strings.Join(otherEnvs, ", "))
	}
	if ef := list(a["env_file"]); len(ef) > 0 {
		var paths []string
		for _, p := range ef {
			if !filepath.IsAbs(p) {
				p = filepath.Join(absCwd, p)
			}
			paths = append(paths, c.rel(p))
		}
		if len(paths) == 1 {
			set("env_file", scalar(paths[0]))
		} else {
			set("env_file", seq(paths))
		}
	}
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		em := mapping()
		for _, k := range keys {
			em.Content = append(em.Content, scalar(k), scalar(env[k]))
		}
		set("environment", em)
	}

	// Everything else: explain what's dropped.
	handled := map[string]bool{
		"name": true, "script": true, "args": true, "interpreter": true, "exec_interpreter": true,
		"interpreter_args": true, "node_args": true, "cwd": true, "namespace": true, "instances": true,
		"exec_mode": true, "autorestart": true, "max_restarts": true, "kill_timeout": true,
		"restart_delay": true, "exp_backoff_restart_delay": true, "max_memory_restart": true,
		"watch": true, "ignore_watch": true, "cron_restart": true, "autostart": true, "env": true,
		"env_file": true, "min_uptime": true, "append_env_to_name": true,
		"appendEnvToName": true, "filter_env": true, "instance_var": true,
	}
	var extra []string
	for k := range a {
		if !handled[k] && !strings.HasPrefix(k, "env_") {
			extra = append(extra, k)
		}
	}
	// Group options that share an explanation (out_file, error_file, …).
	sort.Strings(extra)
	byWhy := map[string][]string{}
	var order []string
	for _, k := range extra {
		why, known := ignored[k]
		if !known {
			why = "unknown pm2 option"
		}
		if byWhy[why] == nil {
			order = append(order, why)
		}
		byWhy[why] = append(byWhy[why], k)
	}
	for _, why := range order {
		note("%s ignored: %s", strings.Join(byWhy[why], ", "), why)
	}
	return name, out, notes
}

// interpreters is pm2's extension → interpreter table.
var interpreters = map[string]string{
	".js": "node", ".cjs": "node", ".mjs": "node",
	".ts": "npx ts-node", ".tsx": "npx ts-node",
	".py": "python3", ".sh": "bash", ".rb": "ruby", ".php": "php",
	".pl": "perl", ".coffee": "coffee",
}

// command builds the shell command pm2 would have run: [interpreter
// [interpreter_args]] script [args].
func (c *converter) command(a map[string]any, script, cwd string) (string, []string) {
	var notes []string
	if script == "" {
		return "", notes
	}
	interp, _ := str(a["interpreter"])
	if interp == "" {
		interp, _ = str(a["exec_interpreter"])
	}
	interpArgs := append(list(a["interpreter_args"]), list(a["node_args"])...)

	if interp == "" {
		interp = interpreters[strings.ToLower(filepath.Ext(script))]
		if strings.HasPrefix(interp, "npx ") {
			notes = append(notes, "TypeScript: pm2 bundles ts-node; this uses `npx ts-node` (or point it at your compiled .js)")
		}
	}
	if interp == "none" {
		interp = ""
	}

	var parts []string
	if interp != "" {
		parts = append(parts, interp)
		parts = append(parts, quoteAll(interpArgs)...)
	} else if len(interpArgs) > 0 {
		notes = append(notes, "interpreter_args/node_args dropped: the script runs directly")
	}

	// A bare relative file run directly needs "./", since sh only searches PATH.
	s := script
	if interp == "" && !strings.Contains(s, "/") {
		if info, err := os.Stat(filepath.Join(cwd, s)); err == nil && !info.IsDir() {
			s = "./" + s
		}
	}
	parts = append(parts, quote(s))

	switch v := a["args"].(type) {
	case string:
		if v != "" {
			parts = append(parts, v) // already a command-line string
		}
	case []any:
		parts = append(parts, quoteAll(list(v))...)
	}
	return strings.Join(parts, " "), notes
}

// rel writes a path relative to the output directory when it lives under it,
// so the sm2.yaml keeps working if the project moves; otherwise absolute.
// Symlinks are resolved first: Node reports __dirname with them resolved
// (macOS /var is really /private/var), so the two sides may differ in form.
func (c *converter) rel(p string) string {
	if c.outDir == "" {
		return p
	}
	if r, ok := under(c.outDir, p); ok {
		return r
	}
	if out, err := filepath.EvalSymlinks(c.outDir); err == nil {
		if r, ok := under(out, realPath(p)); ok {
			return r
		}
	}
	return p
}

func under(dir, p string) (string, bool) {
	r, err := filepath.Rel(dir, p)
	if err != nil || r == ".." || strings.HasPrefix(r, "../") {
		return "", false
	}
	return r, true
}

// realPath resolves symlinks in the longest existing prefix of p (the rest of
// the path may not exist yet).
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	dir, base := filepath.Split(filepath.Clean(p))
	if dir == "" || dir == p {
		return p
	}
	return filepath.Join(realPath(filepath.Clean(dir)), base)
}

// ---- value helpers (decoded JSON/YAML is loosely typed) ----

func str(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case int:
		return strconv.Itoa(t), true
	case bool:
		return strconv.FormatBool(t), true
	}
	return "", false
}

func num(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

func boolean(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		b, err := strconv.ParseBool(t)
		return b, err == nil
	}
	return false, false
}

// list accepts a string or a list of strings.
func list(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := str(x); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// pm2Duration converts a pm2 duration (ms number, or "10s"/"5m"/"1h") into
// an sm2 one.
func pm2Duration(v any) string {
	if n, ok := num(v); ok {
		return msDuration(n)
	}
	s, _ := str(v)
	return s
}

// msDuration renders pm2's millisecond numbers as sm2 durations (5000 → 5s).
func msDuration(ms float64) string {
	n := int64(ms)
	if n%1000 == 0 {
		return fmt.Sprintf("%ds", n/1000)
	}
	return fmt.Sprintf("%dms", n)
}

var plain = regexp.MustCompile(`^[A-Za-z0-9_./:=@%+,-]+$`)

// quote makes s safe as one shell word.
func quote(s string) string {
	if plain.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = quote(s)
	}
	return out
}

// ---- yaml nodes (to control key order and attach comments) ----

func mapping() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode} }

func scalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func intNode(n int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(n)}
}

func boolNode(b bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(b)}
}

func seq(ss []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, s := range ss {
		n.Content = append(n.Content, scalar(s))
	}
	return n
}
