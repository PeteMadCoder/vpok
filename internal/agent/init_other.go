//go:build !linux

package agent

import "os"

func InitGuest(cfg *Config) error {
	return nil
}

func MountPseudofs() error {
	return nil
}

func MountVolumes(mounts []VolumeMount) error {
	return nil
}

func SetupLoopback() error {
	return nil
}

func Poweroff() {
	os.Exit(0)
}
