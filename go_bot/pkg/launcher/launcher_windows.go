//go:build windows

package launcher

import (
	"context"
	"log"

	"wxbot/pkg/config"
	"wxbot/pkg/i18n"
)

// EnsureRuntime leaves Windows process/injection management to the existing
// Windows deployment. The configurable X11/LD_PRELOAD launcher is Linux-only.
func EnsureRuntime(ctx context.Context, cfg *config.Config, catalog *i18n.Catalog, managers ...*LoginManager) error {
	if cfg.Launcher.AutoStart {
		log.Print(catalog.Text("launcher.linux_only"))
	}
	if len(managers) != 0 && managers[0] != nil {
		managers[0].Update("waiting_scan", "", 0)
	}
	return ctx.Err()
}
