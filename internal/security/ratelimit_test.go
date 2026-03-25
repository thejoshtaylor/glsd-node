package security

import (
	"sync"
	"testing"
)

func TestProjectRateLimiterAllow(t *testing.T) {
	// 3 requests per 60 seconds
	prl := NewProjectRateLimiter(3, 60)
	project := "my-project"
	for i := 0; i < 3; i++ {
		if !prl.Allow(project) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if prl.Allow(project) {
		t.Fatal("4th request should be rejected after burst exhausted")
	}
}

func TestProjectRateLimiterPerProject(t *testing.T) {
	prl := NewProjectRateLimiter(2, 60)
	// Exhaust project-a
	prl.Allow("project-a")
	prl.Allow("project-a")
	if prl.Allow("project-a") {
		t.Fatal("project-a 3rd request should be rejected")
	}
	// project-b must still work
	if !prl.Allow("project-b") {
		t.Fatal("project-b should be independent of project-a")
	}
}

func TestProjectRateLimiterConcurrent(t *testing.T) {
	prl := NewProjectRateLimiter(100, 1)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prl.Allow("concurrent-project")
		}()
	}
	wg.Wait()
	// Reaching here without panic = pass
}
