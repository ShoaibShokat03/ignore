//go:build !windows

package tray

import (
	"context"
	"log/slog"
)

type passiveTray struct {
	log *slog.Logger
}

func newController(log *slog.Logger, actions Actions) Controller {
	return &passiveTray{log: log}
}

func (t *passiveTray) Run(ctx context.Context) {
	t.log.Info("tray controller started", "mode", "passive")
	<-ctx.Done()
}

func (t *passiveTray) Stop() {}
