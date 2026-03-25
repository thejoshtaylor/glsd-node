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
