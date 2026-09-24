package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PeteMadCoder/vpok/internal/builder"
	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/store"
)

const defaultStoreDirName = ".vpok/store"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	args := os.Args[2:]

	var err error
	switch subcommand {
	case "keygen":
		err = runKeygen(args)
	case "validate":
		err = runValidate(args)
	case "build":
		err = runBuild(args)
	case "inspect":
		err = runInspect(args)
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: vpok <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("	keygen	 	Generates a new Ed25519 signing keypair")
	fmt.Println("	validate	Validate a vpok.build.toml or vpok.deploy.toml file")
	fmt.Println("	build		Build a package from vpok.build.toml and store into CAS")
	fmt.Println("	inspect		Inspect a manifest in the CAS store by digest")
}

func getDefaultStorePath() string {
	if custom := os.Getenv("VPOK_STORE"); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".vpok", "store")
	}
	return filepath.Join(home, defaultStoreDirName)
}

// vpok keygen
func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	outDir := fs.String("out", "", "Output directory for keys (default: ~/.vpok/keys)")
	fs.Parse(args)

	targetDir := *outDir

	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not determine home directory: %w", err)
		}
		targetDir = filepath.Join(home, ".vpok", "keys")
	}

	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	pub, priv, err := manifest.GenerateKeyPair()
	if err != nil {
		return err
	}

	privHex := hex.EncodeToString(priv)
	pubHex := hex.EncodeToString(pub)

	privPath := filepath.Join(targetDir, "publisher.key")
	pubPath := filepath.Join(targetDir, "publisher.pub")

	if err := os.WriteFile(privPath, []byte(privHex+"\n"), 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}
	if err := os.WriteFile(pubPath, []byte(pubHex+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	fmt.Printf("Generated Ed25519 keypair:\n")
	fmt.Printf("	Private key:   %s (mode 0600)\n", privPath)
	fmt.Printf("	Public key:    %s\n", pubPath)
	fmt.Printf("	Public Key ID: %s\n", pubHex)
	return nil
}

// vpok validate
func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	buildFile := fs.String("build", "", "Path to vpok.build.toml file to validate")
	deployFile := fs.String("deploy", "", "Path to vpok.deploy.toml file to validate")
	fs.Parse(args)

	// Fallback to positional argument if flags are not provided
	if *buildFile == "" && *deployFile == "" {
		if fs.NArg() == 0 {
			return fmt.Errorf("please specify a file to validate using --build, --deploy, or as an argument")
		}
		target := fs.Arg(0)
		if strings.Contains(target, "deploy") {
			*deployFile = target
		} else {
			*buildFile = target
		}
	}

	if *buildFile != "" {
		spec, err := manifest.LoadBuild(*buildFile)
		if err != nil {
			return fmt.Errorf("failed to parse build file: %w", err)
		}
		if err := manifest.ValidateBuild(spec); err != nil {
			return fmt.Errorf("build validation failed: %w", err)
		}

		fmt.Printf("Build specification is valid: %s (package: %s:%s)\n", *buildFile, spec.Metadata.Name, spec.Metadata.Version)
	}

	if *deployFile != "" {
		spec, err := manifest.LoadDeploy(*deployFile)
		if err != nil {
			return fmt.Errorf("failed to parse deploy file: %w", err)
		}
		if err := manifest.ValidateDeploy(spec); err != nil {
			return fmt.Errorf("deploy validation failed: %w", err)
		}
		fmt.Printf("Deploy specification is valid: %s (target package: %s:%s)\n", *deployFile, spec.Metadata.Name, spec.Package.Version)
	}

	return nil
}

// vpok build
func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	specPath := fs.String("f", "vpok.build.toml", "Path to build spec file")
	contextDir := fs.String("C", ".", "Build context directory")
	storePath := fs.String("store", getDefaultStorePath(), "Path to CAS store")
	keyPath := fs.String("key", "", "Path to private key for signing (optional)")
	fs.Parse(args)

	spec, err := manifest.LoadBuild(*specPath)
	if err != nil {
		return fmt.Errorf("failed to read build spec: %w", err)
	}

	casStore, err := store.NewCASStore(*storePath)
	if err != nil {
		return fmt.Errorf("failed to initialize store at %s: %w", *storePath, err)
	}

	var privKey ed25519.PrivateKey
	if *keyPath != "" {
		keyBytes, err := os.ReadFile(*keyPath)
		if err != nil {
			return fmt.Errorf("failed to read signing key: %w", err)
		}
		rawHex := strings.TrimSpace(string(keyBytes))
		decoded, err := hex.DecodeString(rawHex)
		if err != nil {
			return fmt.Errorf("invalid hex encoding in private key file: %w", err)
		}
		if len(decoded) != ed25519.PrivateKeySize {
			return fmt.Errorf("invalid private key size: expected %d bytes, got %d", ed25519.PrivateKeySize, len(decoded))
		}
		privKey = ed25519.PrivateKey(decoded)
	}

	b := builder.NewBuilder(casStore, privKey)
	fmt.Printf("Building package %s:%s from %s...\n", spec.Metadata.Name, spec.Metadata.Version, *specPath)

	m, digest, err := b.Build(context.Background(), spec, *contextDir)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Println("Build succeeded.")
	fmt.Printf("Manifest Digest: %s\n", digest)
	fmt.Printf("Layers (%d):\n", len(m.Layers))
	for i, layer := range m.Layers {
		fmt.Printf("	[%d] %s (%d bytes, mediaType: %s)\n", i, layer.Digest, layer.Size, layer.MediaType)
	}
	if len(m.Signatures) > 0 {
		fmt.Printf("Signatures (%d):\n", len(m.Signatures))
		for _, s := range m.Signatures {
			fmt.Printf("	Publisher: %s (Key ID: %s)\n", s.Publisher, s.KeyID)
		}
	} else {
		fmt.Println("Warning: package was built without a signature (use --key to sign).")
	}

	return nil
}

// vpok inspect
func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	storePath := fs.String("store", getDefaultStorePath(), "Path to CAS store")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("manifest digest required (e.g. vpok inspect sha256:...)")
	}

	digestStr := fs.Arg(0)
	casStore, err := store.NewCASStore(*storePath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}

	m, err := casStore.GetManifest(context.Background(), store.Digest(digestStr))
	if err != nil {
		return fmt.Errorf("failed to retrieve manifest %s: %w", digestStr, err)
	}

	formattedJSON, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format manifest JSON: %w", err)
	}

	fmt.Println(string(formattedJSON))
	return nil
}
