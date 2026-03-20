package security

import (
	"testing"
)

// ---- ValidatePath tests ----

func TestValidatePathAllowed(t *testing.T) {
	allowed := []string{"/home/user/projects"}
	if !ValidatePath("/home/user/projects/foo.txt", allowed) {
		t.Fatal("expected path inside allowed dir to be allowed")
	}
}

func TestValidatePathBlocked(t *testing.T) {
	allowed := []string{"/home/user/projects"}
	if ValidatePath("/etc/passwd", allowed) {
		t.Fatal("expected path outside allowed dir to be blocked")
	}
}

// TestValidatePathNormalization verifies that path traversal sequences like
// /../../../ are resolved before comparison, preventing escape from allowed dirs.
func TestValidatePathNormalization(t *testing.T) {
	allowed := []string{"/home/user/projects"}
	// This resolves to /etc/passwd — must be blocked
	if ValidatePath("/home/user/projects/../../../etc/passwd", allowed) {
		t.Fatal("expected traversal path to be blocked after normalization")
	}
}

// TestValidatePathWindows verifies that Windows-style paths are accepted.
func TestValidatePathWindows(t *testing.T) {
	allowed := []string{`C:\Users\me\projects`}
	if !ValidatePath(`C:\Users\me\projects\foo.txt`, allowed) {
		t.Fatal("expected Windows path inside allowed dir to be allowed")
	}
}

// TestValidatePathWindowsTraversal verifies traversal is blocked on Windows paths too.
func TestValidatePathWindowsTraversal(t *testing.T) {
	allowed := []string{`C:\Users\me\projects`}
	if ValidatePath(`C:\Users\me\projects\..\..\..\Windows\System32\cmd.exe`, allowed) {
		t.Fatal("expected Windows traversal path to be blocked after normalization")
	}
}

// ---- CheckCommandSafety tests ----

func TestCheckCommandSafetyBlocked(t *testing.T) {
	patterns := []string{"sudo rm", "rm -rf /", "format c:"}
	safe, matched := CheckCommandSafety("sudo rm -rf /", patterns)
	if safe {
		t.Fatal("expected 'sudo rm -rf /' to be blocked")
	}
	if matched == "" {
		t.Fatal("expected non-empty matched pattern")
	}
}

func TestCheckCommandSafetyAllowed(t *testing.T) {
	patterns := []string{"sudo rm", "rm -rf /", "format c:"}
	safe, matched := CheckCommandSafety("ls -la", patterns)
	if !safe {
		t.Fatalf("expected 'ls -la' to be safe, got matched=%q", matched)
	}
	if matched != "" {
		t.Fatalf("expected empty matched pattern for safe command, got %q", matched)
	}
}

// TestCheckCommandSafetyCaseInsensitive verifies that matching is case-insensitive.
func TestCheckCommandSafetyCaseInsensitive(t *testing.T) {
	patterns := []string{"format c:"}
	safe, matched := CheckCommandSafety("FORMAT C:", patterns)
	if safe {
		t.Fatalf("expected 'FORMAT C:' to match pattern 'format c:', safe=%v matched=%q", safe, matched)
	}
}

// ---- IsAuthorized tests ----

func TestIsAuthorizedYes(t *testing.T) {
	allowed := []int64{123, 456}
	if !IsAuthorized(123, 999, allowed) {
		t.Fatal("expected userID=123 to be authorized")
	}
}

func TestIsAuthorizedNo(t *testing.T) {
	allowed := []int64{123, 456}
	if IsAuthorized(789, 999, allowed) {
		t.Fatal("expected userID=789 to be unauthorized")
	}
}

// TestIsAuthorizedChannelIDAccepted verifies the function signature accepts
// channelID without compilation error, ensuring Phase 2 forward-compatibility.
func TestIsAuthorizedChannelIDAccepted(t *testing.T) {
	// If this test compiles and runs, channelID is accepted.
	// Phase 1 ignores it; Phase 2 will use it for per-channel membership checks.
	result := IsAuthorized(int64(100), int64(200), []int64{100})
	if !result {
		t.Fatal("expected userID=100 with channelID=200 to be authorized")
	}

	// Different channelID should not affect result in Phase 1
	result2 := IsAuthorized(int64(100), int64(999), []int64{100})
	if !result2 {
		t.Fatal("expected userID=100 with different channelID to still be authorized in Phase 1")
	}
}

func TestIsAuthorizedEmptyList(t *testing.T) {
	if IsAuthorized(123, 0, []int64{}) {
		t.Fatal("expected empty allowedUsers to deny all")
	}
}
