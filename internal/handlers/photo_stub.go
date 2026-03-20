package handlers

// photo_stub.go provides minimal stubs for photo handler functions so the package
// compiles while photo.go (Plan 03-02) is being developed in parallel.
// This file will be REPLACED by the real photo.go from Plan 03-02.

import (
	"fmt"
	"strings"
	"sync"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"golang.org/x/time/rate"

	"github.com/user/gsd-tele-go/internal/audit"
	"github.com/user/gsd-tele-go/internal/config"
	"github.com/user/gsd-tele-go/internal/project"
	"github.com/user/gsd-tele-go/internal/session"
)

// buildSinglePhotoPrompt constructs the prompt text for a single photo.
func buildSinglePhotoPrompt(path string, caption string) string {
	if caption != "" {
		return fmt.Sprintf("[Photo: %s]\n\n%s", path, caption)
	}
	return fmt.Sprintf("[Photo: %s]", path)
}

// buildAlbumPrompt constructs the prompt text for a photo album.
func buildAlbumPrompt(paths []string, caption string) string {
	var sb strings.Builder
	sb.WriteString("[Photos:")
	for i, p := range paths {
		sb.WriteString(fmt.Sprintf("\n%d. %s", i+1, p))
	}
	sb.WriteString("]")
	if caption != "" {
		sb.WriteString("\n\n" + caption)
	}
	return sb.String()
}

// HandlePhoto is a stub that will be replaced by Plan 03-02's photo.go.
func HandlePhoto(
	tgBot *gotgbot.Bot,
	ctx *ext.Context,
	store *session.SessionStore,
	cfg *config.Config,
	auditLog *audit.Logger,
	persist *session.PersistenceManager,
	wg *sync.WaitGroup,
	mappings *project.MappingStore,
	globalLimiter *rate.Limiter,
) error {
	return nil
}
