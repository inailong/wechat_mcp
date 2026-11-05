//go:build linux

package actuator

// NewActuator creates an OS-appropriate message actuator for Linux.
func NewActuator() Actuator {
	return NewX11Actuator()
}
