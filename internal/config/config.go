// Package config provides environment parsing, path resolution, and constants
// for the gsd-tele-go node.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// Session and queue sizing constants.
const (
	MaxSessionHistory = 5
	SessionQueueSize  = 5
)

// BlockedPatterns contains dangerous command patterns that the node will reject.
var BlockedPatterns = []string{
	"rm -rf /",
	"rm -rf ~",
	"rm -rf $HOME",
	"rm -rf %USERPROFILE%",
	"sudo rm",
	":(){ :|:& };:",
	"> /dev/sd",
	"mkfs.",
	"dd if=",
	"format c:",
	"del /s /q c:",
}

// Config holds all configuration values for the node.
type Config struct {
	// WorkingDir is the default working directory for Claude sessions.
	WorkingDir string

	// ClaudeCLIPath is the resolved path to the claude CLI binary.
	ClaudeCLIPath string

	// AllowedPaths is the list of directories Claude is allowed to access.
	AllowedPaths []string

	// RateLimitEnabled controls whether per-channel rate limiting is active.
	RateLimitEnabled bool

	// RateLimitRequests is the number of requests allowed per window.
	RateLimitRequests int

	// RateLimitWindow is the rate limit window duration in seconds.
	RateLimitWindow int

	// AuditLogPath is the path to the append-only audit log file.
	AuditLogPath string

	// DataDir is the directory for runtime JSON files.
	DataDir string

	// SafetyPrompt is the system prompt constraining Claude to safe operations.
	SafetyPrompt string
}

// Load reads configuration from environment variables (and .env if present).
// All variables have documented defaults.
func Load() (*Config, error) {
	// Load .env file if present; ignore error if missing
	_ = godotenv.Load()

	cfg := &Config{}

	// --- CLAUDE_WORKING_DIR (default: home dir) ---
	cfg.WorkingDir = os.Getenv("CLAUDE_WORKING_DIR")
	if cfg.WorkingDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to determine home directory: %w", err)
		}
		cfg.WorkingDir = home
	}

	// --- CLAUDE_CLI_PATH (with LookPath fallback) ---
	cfg.ClaudeCLIPath = resolveClaudeCLIPath()
	log.Info().Str("claude_cli_path", cfg.ClaudeCLIPath).Msg("resolved Claude CLI path")

	// --- ALLOWED_PATHS (default: [WorkingDir]) ---
	allowedPathsStr := os.Getenv("ALLOWED_PATHS")
	if allowedPathsStr != "" {
		for _, p := range strings.Split(allowedPathsStr, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.AllowedPaths = append(cfg.AllowedPaths, p)
			}
		}
	}
	if len(cfg.AllowedPaths) == 0 {
		cfg.AllowedPaths = []string{cfg.WorkingDir}
	}

	// --- RATE_LIMIT_ENABLED (default: true) ---
	rateLimitEnabledStr := os.Getenv("RATE_LIMIT_ENABLED")
	if rateLimitEnabledStr == "" {
		cfg.RateLimitEnabled = true
	} else {
		cfg.RateLimitEnabled = strings.ToLower(rateLimitEnabledStr) == "true"
	}

	// --- RATE_LIMIT_REQUESTS (default: 20) ---
	cfg.RateLimitRequests = 20
	if v := os.Getenv("RATE_LIMIT_REQUESTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid RATE_LIMIT_REQUESTS: %q", v)
		}
		cfg.RateLimitRequests = n
	}

	// --- RATE_LIMIT_WINDOW (default: 60) ---
	cfg.RateLimitWindow = 60
	if v := os.Getenv("RATE_LIMIT_WINDOW"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid RATE_LIMIT_WINDOW: %q", v)
		}
		cfg.RateLimitWindow = n
	}

	// --- AUDIT_LOG_PATH (default: $TEMP/gsd-node-audit.log) ---
	cfg.AuditLogPath = os.Getenv("AUDIT_LOG_PATH")
	if cfg.AuditLogPath == "" {
		cfg.AuditLogPath = filepath.Join(os.TempDir(), "gsd-node-audit.log")
	}

	// --- DATA_DIR (default: ./data) ---
	cfg.DataDir = os.Getenv("DATA_DIR")
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}

	// --- SafetyPrompt (built from AllowedPaths) ---
	cfg.SafetyPrompt = BuildSafetyPrompt(cfg.AllowedPaths)

	return cfg, nil
}

// resolveClaudeCLIPath resolves the claude CLI binary path using the priority order:
// 1. CLAUDE_CLI_PATH env var (if set)
// 2. exec.LookPath("claude")
// 3. Fallback to "claude" literal
func resolveClaudeCLIPath() string {
	if v := os.Getenv("CLAUDE_CLI_PATH"); v != "" {
		return v
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	return "claude"
}

// BuildSafetyPrompt builds the system prompt constraining Claude to safe file operations.
// Exported so per-project WorkerConfig can call it with per-project AllowedPaths.
func BuildSafetyPrompt(paths []string) string {
	var pathsList strings.Builder
	for _, p := range paths {
		pathsList.WriteString("   - ")
		pathsList.WriteString(p)
		pathsList.WriteString(" (and subdirectories)\n")
	}

	return fmt.Sprintf(`
CRITICAL SAFETY RULES FOR GSD NODE:

1. NEVER delete, remove, or overwrite files without EXPLICIT confirmation from the user.
   - If user asks to delete something, respond: "Are you sure you want to delete [file]? Reply 'yes delete it' to confirm."
   - Only proceed with deletion if user replies with explicit confirmation like "yes delete it", "confirm delete"
   - This applies to: rm, trash, unlink, shred, or any file deletion

2. You can ONLY access files in these directories:
%s   - REFUSE any file operations outside these paths

3. NEVER run dangerous commands like:
   - rm -rf (recursive force delete)
   - Any command that affects files outside allowed directories
   - Commands that could damage the system

4. For any destructive or irreversible action, ALWAYS ask for confirmation first.
`, pathsList.String())
}

// FilteredEnv returns os.Environ() with any CLAUDECODE= entries removed.
// This prevents the "nested session" error when running inside Claude Code.
func FilteredEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
