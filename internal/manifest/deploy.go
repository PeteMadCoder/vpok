package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// DeploySpec is the parsed form of vpok.deploy.toml
type DeploySpec struct {
	APIVersion string               `toml:"apiVersion"`
	Kind       string               `toml:"kind"`
	Metadata   DeployMetadata       `toml:"metadata"`
	Package    DeployPackage        `toml:"package"`
	Resources  DeployResources      `toml:"resources"`
	Storage    []DeployStorage      `toml:"storage"`
	Network    *DeployNetwork       `toml:"network"`
	Devices    *DeployDevices       `toml:"devices"`
	Display    *DeployDisplay       `toml:"display"`
	Audio      *DeployAudio         `toml:"audio"`
	Secrets    []DeploySecret       `toml:"secrets"`
	Env        map[string]string    `toml:"env"`
	Restart    *DeployRestart       `toml:"restart"`
	Shutdown   *DeployShutdown      `toml:"shutdown"`
	Service    *DeployService       `toml:"service"`
	License    *DeployLicense       `toml:"license"`
}

type DeployMetadata struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

type DeployPackage struct {
	Source  string               `toml:"source"`
	Version string               `toml:"version"`
	Digest  string               `toml:"digest"`
	Verify  *DeployPackageVerify `toml:"verify"`
}

type DeployPackageVerify struct {
	Signature string `toml:"signature"`
	Publisher string `toml:"publisher"`
}

type DeployResources struct {
	Memory string `toml:"memory"`
	CPUs   int    `toml:"cpus"`
	PIDs   int    `toml:"pids"`
	Disk   string `toml:"disk"`
}

type DeployStorage struct {
	Name       string `toml:"name"`
	GuestPath  string `toml:"guestPath"`
	Type       string `toml:"type"`
	Size       string `toml:"size"`
	Persistent *bool  `toml:"persistent"`
	HostPath   string `toml:"hostPath"`
	Mode       string `toml:"mode"`
}

type DeployNetwork struct {
	Mode  string               `toml:"mode"`
	DNS   []string             `toml:"dns"`
	Allow []DeployNetworkAllow `toml:"allow"`
}

type DeployNetworkAllow struct {
	Host string `toml:"host"`
	CIDR string `toml:"cidr"`
	Port int    `toml:"port"`
}

type DeployDevices struct {
	Webcam     bool `toml:"webcam"`
	Microphone bool `toml:"microphone"`
	GPU        bool `toml:"gpu"`
}

type DeployDisplay struct {
	Enabled  bool   `toml:"enabled"`
	Protocol string `toml:"protocol"`
}

type DeployAudio struct {
	Enabled bool `toml:"enabled"`
}

type DeploySecret struct {
	Name   string `toml:"name"`
	Source string `toml:"source"`
}

type DeployRestart struct {
	Policy      string `toml:"policy"`
	MaxAttempts int    `toml:"maxAttempts"`
	Backoff     string `toml:"backoff"`
}

type DeployShutdown struct {
	GracePeriod string `toml:"gracePeriod"`
}

type DeployService struct {
	Publish []DeployServicePublish `toml:"publish"`
	Update  *DeployServiceUpdate   `toml:"update"`
}

type DeployServicePublish struct {
	Name     string `toml:"name"`
	HostPort int    `toml:"hostPort"`
}

type DeployServiceUpdate struct {
	Strategy          string `toml:"strategy"`
	RollbackOnFailure *bool  `toml:"rollbackOnFailure"`
}

type DeployLicense struct {
	Document   string `toml:"document"`
	Activation string `toml:"activation"`
	Server     string `toml:"server"`
}

// LoadDeploy reads a deploy spec from path and rejects unknown fields.
func LoadDeploy(path string) (*DeploySpec, error) {
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

// LoadDeployString decodes a deploy spec from raw TOML text and rejects unknown fields.
func LoadDeployString(data string) (*DeploySpec, error) {
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
