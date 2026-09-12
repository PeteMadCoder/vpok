package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// DeploySpec is the parsed form of vpok.deploy.toml
// Fill in the fields as you go
type DeploySpec struct {
	APIVersion string   `toml:"apiVersion"`
	Kind       string   `toml:"kind"`
	Metadata   Metadata `toml:"metadata"`
}

type Metadata struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

// Load reads a deploy spec from path and rejects unknown fields.
func Load(path string) (*DeploySpec, error) {
	var spec DeploySpec

	md, err := toml.Decode(path, &spec)
	if err != nil {
		return nil, err
	}

	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown fields: %v", undecoded)
	}

	return &spec, nil
}
