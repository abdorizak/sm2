// Package ipc defines the JSON wire protocol between the sm2 CLI and agent.
package ipc

// Action names carried in a Request.
const (
	ActionPing       = "ping"
	ActionStart      = "start"
	ActionStop       = "stop"
	ActionRestart    = "restart"
	ActionStatus     = "status"
	ActionReload     = "reload"
	ActionDelete     = "delete"
	ActionReset      = "reset"
	ActionDescribe   = "describe"
	ActionSignal     = "signal"
	ActionSave       = "save"
	ActionResurrect  = "resurrect"
	ActionNotifySet  = "notify_set"
	ActionNotifyGet  = "notify_get"
	ActionNotifyTest = "notify_test"
	ActionRotateSet  = "rotate_set"
	ActionRotateGet  = "rotate_get"
	ActionRotateNow  = "rotate_now"
)

// DiscordConfig is the Discord webhook setting carried over the wire.
type DiscordConfig struct {
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
}

// LogRotateConfig is the log-rotation setting carried over the wire and
// persisted to logrotate.json.
type LogRotateConfig struct {
	Enabled      bool   `json:"enabled"`
	MaxSizeBytes int64  `json:"max_size_bytes"`
	Retain       int    `json:"retain"`
	Compress     bool   `json:"compress"`
	Interval     string `json:"interval,omitempty"` // optional cron, e.g. "0 0 * * *"
}

// Request is a single command sent from the CLI to the agent.
type Request struct {
	Action     string   `json:"action"`
	App        *AppSpec `json:"app,omitempty"`
	Name       string   `json:"name,omitempty"`
	Namespace  string   `json:"namespace,omitempty"`
	Signal     string   `json:"signal,omitempty"`
	ConfigPath string   `json:"config_path,omitempty"`
	UpdateEnv  bool     `json:"update_env,omitempty"` // refresh the base env on restart
	Env        []string `json:"env,omitempty"`        // caller's environment, for UpdateEnv

	Discord   *DiscordConfig   `json:"discord,omitempty"`    // for notify_set
	LogRotate *LogRotateConfig `json:"log_rotate,omitempty"` // for rotate_set
}

// AppSpec describes an application the agent should manage.
type AppSpec struct {
	Name       string            `json:"name"`
	Command    string            `json:"command"`
	Dir        string            `json:"dir,omitempty"`
	Namespace  string            `json:"namespace,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Restart    string            `json:"restart,omitempty"` // always | on-failure | never
	MaxRetries int               `json:"max_retries,omitempty"`

	// Lifecycle tuning
	NoAutostart    bool `json:"no_autostart,omitempty"`
	KillTimeoutMs  int  `json:"kill_timeout_ms,omitempty"`
	RestartDelayMs int  `json:"restart_delay_ms,omitempty"`
	ExpBackoff     bool `json:"exp_backoff,omitempty"`

	// Restart triggers
	MaxMemoryBytes int64    `json:"max_memory_bytes,omitempty"`
	Watch          bool     `json:"watch,omitempty"`
	WatchPaths     []string `json:"watch_paths,omitempty"`
	IgnoreWatch    []string `json:"ignore_watch,omitempty"`
	CronRestart    string   `json:"cron_restart,omitempty"`
}

// Response is the agent's reply to a Request.
type Response struct {
	OK        bool             `json:"ok"`
	Error     string           `json:"error,omitempty"`
	Apps      []AppStatus      `json:"apps,omitempty"`
	Detail    *AppDetail       `json:"detail,omitempty"`
	Discord   *DiscordConfig   `json:"discord,omitempty"`    // for notify_get
	LogRotate *LogRotateConfig `json:"log_rotate,omitempty"` // for rotate_get/set
	Rotated   int              `json:"rotated,omitempty"`    // count rotated by rotate_now
}

// AppStatus is a point-in-time snapshot of one managed app.
type AppStatus struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace,omitempty"`
	State       string  `json:"state"`
	PID         int     `json:"pid"`
	Restarts    int     `json:"restarts"`
	Uptime      string  `json:"uptime"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes int64   `json:"memory_bytes"`
	User        string  `json:"user,omitempty"`
	Watching    bool    `json:"watching"`
	Command     string  `json:"command"`
}

// AppDetail is the full picture of one app, returned by describe.
type AppDetail struct {
	Status    AppStatus `json:"status"`
	Spec      AppSpec   `json:"spec"`
	StdoutLog string    `json:"stdout_log"`
	StderrLog string    `json:"stderr_log"`
}
