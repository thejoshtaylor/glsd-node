package config

import (
	"os"
	"strings"
	"testing"
)

// setEnv is a helper that sets environment variables and returns a cleanup function.
func setEnv(t *testing.T, pairs map[string]string) {
	t.Helper()
	for k, v := range pairs {
		t.Setenv(k, v)
	}
}

// TestLoadConfig verifies that Load populates fields from env vars.
func TestLoadConfig(t *testing.T) {
	setEnv(t, map[string]string{
		"CLAUDE_WORKING_DIR": "/tmp/test-work",
		"ALLOWED_PATHS":      "/tmp/test-work,/tmp/docs",
		"DATA_DIR":           "/tmp/data",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.WorkingDir != "/tmp/test-work" {
		t.Errorf("WorkingDir = %q, want %q", cfg.WorkingDir, "/tmp/test-work")
	}

	if len(cfg.AllowedPaths) != 2 {
		t.Errorf("AllowedPaths length = %d, want 2", len(cfg.AllowedPaths))
	}

	if cfg.DataDir != "/tmp/data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/tmp/data")
	}
}

// TestLoadConfigDefaults verifies that default values are applied when optional env vars are unset.
func TestLoadConfigDefaults(t *testing.T) {
	// No required vars — Load should succeed with defaults
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if !cfg.RateLimitEnabled {
		t.Error("RateLimitEnabled should default to true")
	}

	if cfg.RateLimitRequests != 20 {
		t.Errorf("RateLimitRequests = %d, want 20", cfg.RateLimitRequests)
	}

	if cfg.RateLimitWindow != 60 {
		t.Errorf("RateLimitWindow = %d, want 60", cfg.RateLimitWindow)
	}

	if cfg.AuditLogPath == "" {
		t.Error("AuditLogPath should have a default value")
	}

	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "./data")
	}

	// AllowedPaths should default to [WorkingDir]
	if len(cfg.AllowedPaths) == 0 {
		t.Error("AllowedPaths should default to at least one path")
	}
}

// TestResolvePaths verifies that CLAUDE_CLI_PATH env var takes precedence over LookPath.
func TestResolvePaths(t *testing.T) {
	t.Setenv("CLAUDE_CLI_PATH", "/custom/path/to/claude")

	resolved := resolveClaudeCLIPath()
	if resolved != "/custom/path/to/claude" {
		t.Errorf("resolveClaudeCLIPath() = %q, want %q", resolved, "/custom/path/to/claude")
	}
}

// TestResolvePathsFallback verifies fallback to "claude" literal when env and LookPath both fail.
func TestResolvePathsFallback(t *testing.T) {
	// Clear the env var; LookPath will also likely fail in test env
	t.Setenv("CLAUDE_CLI_PATH", "")

	resolved := resolveClaudeCLIPath()
	// Result should be either found by LookPath (any non-empty string) or the "claude" literal fallback
	if resolved == "" {
		t.Error("resolveClaudeCLIPath() should never return empty string")
	}
}

// TestFilteredEnv verifies that CLAUDECODE= entries are removed from the environment.
func TestFilteredEnv(t *testing.T) {
	t.Setenv("CLAUDECODE", "test-value")
	t.Setenv("CLAUDECODE_EXTRA", "should-also-be-filtered")

	filtered := FilteredEnv()

	for _, e := range filtered {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			t.Errorf("FilteredEnv() should not contain CLAUDECODE=, but found: %q", e)
		}
	}
}

// TestFilteredEnvPreservesOthers verifies that non-CLAUDECODE env vars are preserved.
func TestFilteredEnvPreservesOthers(t *testing.T) {
	const testKey = "GSD_TELE_TEST_VAR"
	const testVal = "keep-me"
	t.Setenv(testKey, testVal)
	t.Setenv("CLAUDECODE", "remove-me")

	filtered := FilteredEnv()

	found := false
	for _, e := range filtered {
		if e == testKey+"="+testVal {
			found = true
		}
	}
	if !found {
		t.Errorf("FilteredEnv() should preserve %s=%s", testKey, testVal)
	}
}

// TestBuildSafetyPrompt verifies the safety prompt contains expected content.
func TestBuildSafetyPrompt(t *testing.T) {
	paths := []string{"/path/a", "/path/b"}
	prompt := BuildSafetyPrompt(paths)

	if !strings.Contains(prompt, "/path/a") {
		t.Error("safety prompt should contain /path/a")
	}
	if !strings.Contains(prompt, "/path/b") {
		t.Error("safety prompt should contain /path/b")
	}
	if !strings.Contains(prompt, "CRITICAL SAFETY RULES") {
		t.Error("safety prompt should contain CRITICAL SAFETY RULES header")
	}
}

// TestConstants verifies exported constants have expected values.
func TestConstants(t *testing.T) {
	if MaxSessionHistory != 5 {
		t.Errorf("MaxSessionHistory = %d, want 5", MaxSessionHistory)
	}
	if SessionQueueSize != 5 {
		t.Errorf("SessionQueueSize = %d, want 5", SessionQueueSize)
	}
}

// TestBlockedPatterns verifies the blocked patterns slice is populated.
func TestBlockedPatterns(t *testing.T) {
	if len(BlockedPatterns) == 0 {
		t.Error("BlockedPatterns should not be empty")
	}

	// Verify specific dangerous patterns are present
	mustContain := []string{"rm -rf /", "sudo rm", "format c:", "del /s /q c:"}
	for _, pattern := range mustContain {
		found := false
		for _, bp := range BlockedPatterns {
			if bp == pattern {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("BlockedPatterns should contain %q", pattern)
		}
	}
}

// Ensure os is used (needed for UserHomeDir in Load).
var _ = os.UserHomeDir
