package manifest

// Manifest represents a signed, cannonical package manifest
type Manifest struct {
	APIVersion  string            `json:"apiVersion"`
	Kind        string            `json:"kind"`
	Metadata    BuildMetadata     `json:"metadata"`
	Base        Base              `json:"base"`
	Layers      []LayerRef        `json:"layers"`
	BuildSpec   BuildSpec         `json:"buildSpec"`
	EntryPoint  EntryPoint        `json:"entrypoint"`
	Requires    Requires          `json:"requires"`
	Service     *BuildService     `json:"service,omitempty"`
	Signatures  []Signature       `json:"signatures,omitempty"`
	SBOM        *LayerRef         `json:"sbom,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type LayerRef struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	MediaType string `json:"mediaType,omitempty"`
}

type Signature struct {
	Publisher string `json:"publisher"`
	KeyID     string `json:"keyID"`
	Signature string `json:"signature"`
}
