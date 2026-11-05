//go:build !linux && !windows

package actuator

import (
	"context"
	"errors"
)

type FallbackActuator struct{}

func NewActuator() Actuator {
	return &FallbackActuator{}
}

func (a *FallbackActuator) SetDryRun(dryRun bool) {}

func (a *FallbackActuator) IsDryRun() bool {
	return false
}

func (a *FallbackActuator) SendMessage(ctx context.Context, recipient, content string) error {
	return errors.New("gui message actuator not supported on this operating system")
}
