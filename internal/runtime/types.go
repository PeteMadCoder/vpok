package runtime

import (
	"context"
	"io"
	"time"

	"github.com/PeteMadCoder/vpok/internal/agent"
)

// State represents the current lifecycle state of a VM instance.
type State string

const (
	StateCreated State = "created"
	StateRunning State = "running"
	StateStopped State = "stopped"
	StateFailed  State = "failed"
)

// ExitStatus contains the exit outcome of the VM process.
type ExitStatus struct {
	ExitCode int
	Err      error
}

// Drive defines a block device attached to the VM.
type Drive struct {
	ID       string
	Path     string
	ReadOnly bool
	Format   string // "raw" or "qcow2"
}

// SharedDir defines a host directory shared into the guest via virtio-fs or 9p.
type SharedDir struct {
	Tag      string // Mount tag in guest
	HostPath string // Absolute path on host
	ReadOnly bool
}

// NetworkConfig defines VM network interface parameters.
type NetworkConfig struct {
	Enabled   bool
	TapDevice string
	MAC       string
}

// VMConfig contains all parameters required to launch a guest VM.
type VMConfig struct {
	ID          string
	MemoryMB    int
	VCPUs       int
	KernelPath  string
	InitrdPath  string
	Cmdline     string
	RootDrive   *Drive
	Drives      []Drive
	SharedDirs  []SharedDir
	Network     NetworkConfig
	VsockCID    uint32
	AgentConfig *agent.Config
	LogFile     string
}

// InstanceSummary provides high-level metadata about a VM instance.
type InstanceSummary struct {
	ID        string    `json:"id"`
	State     State     `json:"state"`
	PID       int       `json:"pid"`
	StartTime time.Time `json:"startTime,omitempty"`
	MemoryMB  int       `json:"memoryMb"`
	VCPUs     int       `json:"vcpus"`
}

// Instance represents a manageable virtual machine instance.
type Instance interface {
	ID() string
	State() State
	PID() int
	Start(ctx context.Context) error
	Stop(ctx context.Context, gracePeriod time.Duration) error
	Wait(ctx context.Context) (ExitStatus, error)
	Logs() (io.ReadCloser, error)
}

// Runtime abstracts hypervisor VM management.
type Runtime interface {
	Create(ctx context.Context, cfg VMConfig) (Instance, error)
	Get(ctx context.Context, id string) (Instance, error)
	List(ctx context.Context) ([]InstanceSummary, error)
	Delete(ctx context.Context, id string) error
}
