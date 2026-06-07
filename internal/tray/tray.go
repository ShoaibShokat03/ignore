package tray

import (
	"context"
	"log/slog"
)

type Actions struct {
	Open    func()
	Enable  func()
	Disable func()
	Reload  func()
	Logs    func()
	Exit    func()
}

type Controller interface {
	Run(context.Context)
	Stop()
}

func New(log *slog.Logger, actions Actions) Controller {
	return newController(log, actions)
}
