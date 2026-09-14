package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// DeploySpec is the parsed form of vpok.deploy.toml
type DeploySpec struct {
	APIVersion string            `toml:"apiVersion"`
	Kind       string            `toml:"kind"`
	Metadata   Metadata          `toml:"metadata"`
	Package    Package           `toml:"package"`
	Resources  Resources         `toml:"resources"`
	Storage    []Storage         `toml:"storage"`
	Network    *Network          `toml:"network"`
	Devices    *Devices          `toml:"devices"`
	Display    *Display          `toml:"display"`
	Audio      *Audio            `toml:"audio"`
	Secrets    []Secret          `toml:"secrets"`
	Env        map[string]string `toml:"env"`
	Restart    *Restart          `toml:"restart"`
	Shutdown   *Shutdown         `toml:"shutdown"`
	Service    *DeployService    `toml:"service"`
	License    *License          `toml:"license"`
}

type Metadata struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

type Package struct {
	Source  string         `toml:"source"`
	Version string         `toml:"version"`
	Digest  string         `toml:"digest"`
	Verify  *PackageVerify `toml:"verify"`
}

type PackageVerify struct {
	Signature string `toml:"signature"`
	Publisher string `toml:"publisher"`
}

type Resources struct {
	Memory string `toml:"memory"`
	CPUs   int    `toml:"cpus"`
	PIDs   int    `toml:"pids"`
	Disk   string `toml:"disk"`
}

type Storage struct {
	Name       string `toml:"name"`
	GuestPath  string `toml:"guestPath"`
	Type       string `toml:"type"`
	Size       string `toml:"size"`
	Persistent *bool  `toml:"persistent"`
	HostPath   string `toml:"hostPath"`
	Mode       string `toml:"mode"`
}

type Network struct {
	Mode  string         `toml:"mode"`
	DNS   []string       `toml:"dns"`
	Allow []NetworkAllow `toml:"allow"`
}

type NetworkAllow struct {
	Host string `toml:"mode"`
	CIDR string `toml:"cidr"`
	Port int    `toml:"port"`
}

type Devices struct {
	Webcam     bool `toml:"webcam"`
	Microphone bool `toml:"microphone"`
	GPU        bool `toml:"gpu"`
}

type Display struct {
	Enabled  bool   `toml:"enabled"`
	Protocol string `toml:"protocol"`
}

type Audio struct {
	Enabled bool `toml:"enabled"`
}

type Secret struct {
	Name   string `toml:"name"`
	Source string `toml:"source"`
}

type Restart struct {
	Policy      string `toml:"policy"`
	MaxAttempts int    `toml:"maxAttempts"`
	Backoff     string `toml:"backoff"`
}

type Shutdown struct {
	GracePeriod string `toml:"gracePeriod"`
}

type DeployService struct {
	Publish []ServicePublish `toml:"publish"`
	Update  *ServiceUpdate   `toml:"update"`
}

type ServicePublish struct {
	Name     string `toml:"name"`
	HostPort int    `toml:"hostPort"`
}

type ServiceUpdate struct {
	Strategy          string `toml:"strategy"`
	RollbackOnFailure *bool  `toml:"rollbackOnFailure"`
}

type License struct {
	Document   string `toml:"document"`
	Activation string `toml:"activation"`
	Server     string `toml:"server"`
}

// Load reads a deploy spec from path and rejects unknown fields.
func Load(path string) (*DeploySpec, error) {
	var spec DeploySpec

	md, err := toml.DecodeFile(path, &spec)
	if err != nil {
		return nil, err
	}

	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown fields: %v", undecoded)
	}

	return &spec, nil
}

// LoadString decodes a deploy spec from raw TOML text and rejects unknown fields.
func LoadString(data string) (*DeploySpec, error) {
	var spec DeploySpec

	md, err := toml.Decode(data, &spec)
	if err != nil {
		return nil, err
	}

	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown fields: %v", undecoded)
	}

	return &spec, nil
}
