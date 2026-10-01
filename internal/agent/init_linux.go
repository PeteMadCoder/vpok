//go:build linux

package agent

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"
)

// InitGuest initializes the guest system environment when running as PID 1.
func InitGuest(cfg *Config) error {
	// 1. Set hostname if configured
	if cfg != nil && cfg.Hostname != "" {
		_ = syscall.Sethostname([]byte(cfg.Hostname))
	}

	// 2. Mount declared volumes
	if err := MountPseudofs(); err != nil {
		return fmt.Errorf("failed to mount pseudofilesystems: %w", err)
	}

	// 3. Mount declared volumes
	if cfg != nil && len(cfg.Mounts) > 0 {
		if err := MountVolumes(cfg.Mounts); err != nil {
			log.Printf("warning: some volumes failed to mount: %v", err)
		}
	}

	// 4. Setup loopback interface
	if err := SetupLoopback(); err != nil {
		log.Printf("warning: failed to bring up loopback interface: %v", err)
	}

	return nil
}

// MountPseudofs mounts /proc, /sys, /dev, /dev/pts, /dev/shm, /run, /tmp
func MountPseudofs() error {
	mounts := []struct {
		source string
		target string
		fstype string
		flags  uintptr
		data   string
	}{
		{"proc", "/proc", "proc", syscall.MS_NOSUID | syscall.MS_NOEXEC | syscall.MS_NODEV, ""},
		{"sysfs", "/sys", "sysfs", syscall.MS_NOSUID | syscall.MS_NOEXEC | syscall.MS_NODEV, ""},
		{"devtmpfs", "/dev", "devtmpfs", syscall.MS_NOSUID, "mode=0755"},
		{"devpts", "/dev/pts", "devpts", syscall.MS_NOSUID | syscall.MS_NOEXEC, "gid=5,mode=620"},
		{"tmpfs", "/dev/shm", "tmpfs", syscall.MS_NOSUID | syscall.MS_NOEXEC, "mode=1777"},
		{"tmpfs", "/run", "tmpfs", syscall.MS_NOSUID | syscall.MS_NOEXEC, "mode=0755"},
		{"tmpfs", "/tmp", "tmpfs", syscall.MS_NOSUID | syscall.MS_NOEXEC, "mode=1777"},
	}

	for _, m := range mounts {
		if err := os.MkdirAll(m.target, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", m.target, err)
		}
		if err := syscall.Mount(m.source, m.target, m.fstype, m.flags, m.data); err != nil {
			if err != syscall.EBUSY {
				log.Printf("mount %s on %s (%s) error: %v", m.source, m.target, m.fstype, err)
			}
		}
	}
	return nil
}

// MountVolumes mounts all declared storage volume mounts into the guest.
func MountVolumes(mounts []VolumeMount) error {
	for _, m := range mounts {
		if err := os.MkdirAll(m.GuestPath, 0755); err != nil {
			return fmt.Errorf("failed to create mountpoint %s: %w", m.GuestPath, err)
		}

		fsType := m.FSType
		if fsType == "" {
			fsType = "9p"
		}

		var flags uintptr
		if m.ReadOnly {
			flags |= syscall.MS_RDONLY
		}

		if err := syscall.Mount(m.Tag, m.GuestPath, fsType, flags, m.Options); err != nil {
			log.Printf("failed to mount volume %s (%s) on %s: %v", m.Tag, fsType, m.GuestPath, err)
		}
	}

	return nil
}

// SetupLoopback brings up loopback (lo) network interface using socket ioctl.
func SetupLoopback() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)

	if err != nil {
		return err
	}
	defer syscall.Close(fd)

	var ifr [40]byte
	copy(ifr[:], "lo")
	// IFF_UP = 0x1, IFF_RUNNING = 0x40
	flags := uint16(0x1 | 0x40)
	ifr[16] = byte(flags)
	ifr[17] = byte(flags >> 8)

	// SIOCSIFFLAGS = 0x8914
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(0x8914), uintptr(unsafe.Pointer(&ifr[0])))
	if errno != 0 {
		return errno
	}
	return nil
}

// Poweroff gracefully halts and powers off the Linux VM.
func Poweroff() {
	syscall.Sync()
	_ = syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}
