package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// BuildSpec is the parsed form of vpok.build.toml
type BuildSpec struct {
	APIVersion   string        `toml:"apiVersion"`
	Kind         string        `toml:"kind"`
	Metadata     BuildMetadata `toml:"metadata"`
	Base         Base          `toml:"base"`
	Build        *BuildConfig  `toml:"build"`
	EntryPoint   EntryPoint    `toml:"entrypoint"`
	Requires     Requires      `toml:"requires"`
	Service      *BuildService `toml:"service"`
	Reproducible *Reproducible `toml:"reproducible"`
}

type BuildMetadata struct {
	Name        string `toml:"name"`
	Version     string `toml:"version"`
	Publisher   string `toml:"publisher"`
	Description string `toml:"description"`
	License     string `toml:"license"`
	Homepage    string `toml:"homepage"`
}

type Base struct {
	Distribution string `toml:"distribution"`
	Release      string `toml:"release"`
	Architecture string `toml:"architecture"`
}

type BuildConfig struct {
	Steps []BuildStep `toml:"steps"`
}

type BuildStep struct {
	Action  string            `toml:"action"`
	Command string            `toml:"command"`
	From    string            `toml:"from"`
	To      string            `toml:"to"`
	Mode    string            `toml:"mode"`
	Vars    map[string]string `toml:"vars"`
}

type EntryPoint struct {
	Command    []string `toml:"command"`
	WorkingDir string   `toml:"workingDir"`
}

type Requires struct {
	Resources RequireResources `toml:"resources"`
	DataDirs  []DataDirs       `toml:"dataDirs"`
	Network   RequireNetwork   `toml:"network"`
	Devices   *RequireDevices  `toml:"devices"`
	Display   *RequireDisplay  `toml:"display"`
	Audio     *RequireAudio    `toml:"audio"`
}

type RequireResources struct {
	Memory string `toml:"memory"`
	CPUs   int    `toml:"cpus"`
}

type DataDirs struct {
	Path string `toml:"path"`
}

type RequireNetwork struct {
	Outbound bool `toml:"outbound"`
}

type RequireDevices struct {
	Webcam     bool `toml:"webcam"`
	Microphone bool `toml:"microphone"`
	GPU        bool `toml:"gpu"`
}

type RequireDisplay struct {
	Enabled  bool   `toml:"enabled"`
	Protocol string `toml:"protocol"`
}

type RequireAudio struct {
	Enabled bool `toml:"enabled"`
}

type BuildService struct {
	Ports     []ServicePort   `toml:"ports"`
	Health    *HealthCheck    `toml:"health"`
	Readiness *ReadinessCheck `toml:"readiness"`
}

type ServicePort struct {
	Name          string `toml:"name"`
	ContainerPort int    `toml:"containerPort"`
	Protocol      string `toml:"protocol"`
}

type HealthCheck struct {
	Type     string `toml:"type"`
	Path     string `toml:"path"`
	Port     int    `toml:"port"`
	Interval string `toml:"interval"`
	Timeout  string `toml:"timeout"`
	Failures int    `toml:"failures"`
}

type ReadinessCheck struct {
	Type     string `toml:"type"`
	Path     string `toml:"path"`
	Port     int    `toml:"port"`
	Interval string `toml:"interval"`
}

type Reproducible struct {
	Enabled bool `toml:"enabled"`
	SBOM    bool `toml:"sbom"`
}

// LoadBuild reads a build spec from a file path and rejects unknown fields.
func LoadBuild(path string) (*BuildSpec, error) {
	var spec BuildSpec

	md, err := toml.DecodeFile(path, &spec)
	if err != nil {
		return nil, err
	}

	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("Unknown fields: %v", undecoded)
	}

	return &spec, nil
}

// LoadBuildString reads a build spec from a raw TOML text and rejects unknown fields.
func LoadBuildString(data string) (*BuildSpec, error) {
	var spec BuildSpec

	md, err := toml.Decode(data, &spec)
	if err != nil {
		return nil, err
	}

	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("Unknown fields: %v", undecoded)
	}

	return &spec, nil
}
