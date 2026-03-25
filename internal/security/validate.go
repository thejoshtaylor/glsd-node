package security

import (
	"path/filepath"
	"strings"
)

// ValidatePath reports whether path is within one of the allowedPaths directories.
// Both path and each allowed path are normalized via filepath.Clean and converted to
// forward slashes before comparison, preventing path traversal attacks.
func ValidatePath(path string, allowedPaths []string) bool {
	cleanPath := filepath.ToSlash(filepath.Clean(path))

	for _, allowed := range allowedPaths {
		cleanAllowed := filepath.ToSlash(filepath.Clean(allowed))
		if strings.HasPrefix(cleanPath, cleanAllowed) {
			return true
		}
	}
	return false
}

// CheckCommandSafety checks whether text contains any of the blockedPatterns.
// Matching is case-insensitive substring search.
// Returns (true, "") if the text is safe.
// Returns (false, matchedPattern) if a blocked pattern is found.
func CheckCommandSafety(text string, blockedPatterns []string) (bool, string) {
	lower := strings.ToLower(text)
	for _, pattern := range blockedPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return false, pattern
		}
	}
	return true, ""
}
