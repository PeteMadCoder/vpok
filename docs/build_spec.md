# VPOK Build Specification

The build spec describes how to construct a VPOK package. It is authored by the publisher, embedded in the signed manifest, and used to reproduce the package.

**Audience:** publishers who package software for VPOK.
**Default filename:** `vpok.build.toml`.
**Format:** TOML.
**API version:** `vpok.io/v1`.

---

## Minimal example

```toml
apiVersion = "vpok.io/v1"
kind = "Build"

[metadata]
name = "hello"
version = "0.1.0"
publisher = "example.org"

[base]
distribution = "alpine"
release = "3.20"
architecture = "amd64"

[entrypoint]
command = ["/bin/echo", "hello"]

[requires.resources]
memory = "64MiB"
cpus = 1
```

---

## Complex example

This example packages a CLI tool. Every optional field is shown once. Commented blocks are marked as such.

```toml
apiVersion = "vpok.io/v1"
kind = "Build"

# ---------------------------------------------------------------
# Identity and metadata
# ---------------------------------------------------------------

[metadata]
name = "image-tool"
version = "1.4.0"
publisher = "example.org"
description = "Image processing CLI"
license = "Apache-2.0"
homepage = "https://example.org/image-tool"

# ---------------------------------------------------------------
# Base environment
# ---------------------------------------------------------------

[base]
distribution = "alpine"
release = "3.20"
architecture = "amd64"

# ---------------------------------------------------------------
# Build steps
# ---------------------------------------------------------------

[[build.steps]]
action = "run"
command = "apk add --no-cache imagemagick"

[[build.steps]]
action = "copy"
from = "./bin/image-tool"
to = "/opt/image-tool/bin/image-tool"
mode = "0755"

[[build.steps]]
action = "run"
command = "strip /opt/image-tool/bin/image-tool"

[[build.steps]]
action = "env"
vars = { IMAGE_TOOL_HOME = "/opt/image-tool" }

# ---------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------

[entrypoint]
command = ["/opt/image-tool/bin/image-tool"]
workingDir = "/opt/image-tool"

# ---------------------------------------------------------------
# Requirements
# ---------------------------------------------------------------

[requires.resources]
memory = "512MiB"
cpus = 1

[[requires.dataDirs]]
path = "/var/lib/image-tool"

[requires.network]
outbound = false

[requires.devices]
webcam = false
microphone = false
gpu = false

[requires.display]
enabled = false

[requires.audio]
enabled = false

# ---------------------------------------------------------------
# Reproducibility
# ---------------------------------------------------------------

[reproducible]
enabled = true
sbom = true
```

---

## Top-level fields

| Field        | Type   | Required | Description                                      |
|--------------|--------|----------|--------------------------------------------------|
| `apiVersion` | string | yes      | Always `"vpok.io/v1"` for this version.          |
| `kind`       | string | yes      | Always `"Build"`.                                |

Unknown top-level keys are rejected.

---

## `[metadata]`

Identifies the package.

| Field         | Type   | Required | Description                                                    |
|---------------|--------|----------|----------------------------------------------------------------|
| `name`        | string | yes      | Package name. Lowercase, DNS-safe: `[a-z0-9-]+`.               |
| `version`     | string | yes      | Semantic version: `MAJOR.MINOR.PATCH`. Must be a quoted string.|
| `publisher`   | string | yes      | Signing identity. Must match the key used at publish time.    |
| `description` | string | no       | One-line human-readable description.                          |
| `license`     | string | no       | SPDX license identifier.                                      |
| `homepage`    | string | no       | URL for documentation or project page.                        |

**Note:** `name` and `version` together form the package identity within a registry. Two packages with the same `name` and `version` but different content are considered a collision and will be rejected by the registry.

---

## `[base]`

Selects the base distribution for the guest filesystem.

| Field          | Type   | Required | Description                                              |
|----------------|--------|----------|----------------------------------------------------------|
| `distribution` | string | yes      | Base distribution. `"alpine"` is the only supported value in v1. |
| `release`      | string | yes      | Distribution release. Quoted string, e.g. `"3.20"`.      |
| `architecture` | string | yes      | Target architecture. `"amd64"` and `"arm64"` supported.  |

The base layer is provided by VPOK and cached locally. It is not part of your package's layers, it is referenced by digest in the manifest.

---

## `[[build.steps]]`

An ordered list of steps that transform the base filesystem into the package filesystem. Steps run in the order they appear.

Every step is a **tagged union**: the `action` field selects which other fields are legal. An unknown `action`, or a field that does not belong to the selected action, is a validation error.

### `action = "run"`

Executes a shell command inside the build environment.

| Field     | Type   | Required | Description                              |
|-----------|--------|----------|------------------------------------------|
| `action`  | string | yes      | `"run"`.                                 |
| `command` | string | yes      | Shell command. Executed with `/bin/sh -c`. |

Example:

```toml
[[build.steps]]
action = "run"
command = "apk add --no-cache curl"
```

The command runs as root in the build environment. Network access during build is enabled by default, it is a build-time concern, not a runtime concern, and is not governed by the deploy spec.


### `action = "copy"`

Copies a file or directory from the build context into the package filesystem.

| Field    | Type   | Required | Description                                                  |
|----------|--------|----------|--------------------------------------------------------------|
| `action` | string | yes      | `"copy"`.                                                    |
| `from`   | string | yes      | Path in the build context, relative to the build spec.       |
| `to`     | string | yes      | Absolute path in the package filesystem.                     |
| `mode`   | string | no       | File mode as a quoted octal string, e.g. `"0755"`. Defaults to preserving the source mode. |

`from` must be within the build context. Paths containing `..` that escape the context are rejected.


### `action = "env"`

Sets environment variables for subsequent steps.

| Field    | Type         | Required | Description                                |
|----------|--------------|----------|--------------------------------------------|
| `action` | string       | yes      | `"env"`.                                    |
| `vars`   | inline table | yes      | Map of variable name to string value.       |

Example:

```toml
[[build.steps]]
action = "env"
vars = { PATH = "/usr/local/bin:/usr/bin:/bin" }
```

Variables set here affect only the build. To set variables for the running application, use the `env` field in the deploy spec.

### Supported actions in v1

| Action | Purpose                        |
|--------|--------------------------------|
| `run`  | Execute a shell command.       |
| `copy` | Copy a file or directory.      |
| `env`  | Set build-time variables.      |

Additional actions may be added in future API versions. They will not appear in `vpok.io/v1`, they will require an explicit version bump.

---

## `[entrypoint]`

Defines what runs when the package is started.

| Field        | Type          | Required | Description                                        |
|--------------|---------------|----------|----------------------------------------------------|
| `command`    | array<string> | yes      | The process to execute. First element is the binary. |
| `workingDir` | string        | no       | Working directory. Defaults to `/`.                |

The entrypoint must run in the foreground. VPOK does not daemozine anything. The process is expected to:
- Handle `SIGTERM` and exit cleanly within the deployment's grace period.
- Write logs to stdout or stderr.
- Exit with a nonzero status on failure.

```toml
[entrypoint]
command = ["/opt/server/bin/server", "--config", "/etc/server.toml"]
workingDir = "/opt/server"
```

## `[requires]`

What the package **needs** in order to run. These are declared by the publisher. They are not grants.

For every field in `requires`, the operator must provide a matching value in their deploy spec that is a least as permissive. If a requirement is not satisfied, the deployment fails to start with a precise error naming the missing grant.

### `[requires.resources]`

Minimum resource requirements.

| Field    | Type   | Required | Description                                        |
|----------|--------|----------|----------------------------------------------------|
| `memory` | string | yes      | Memory size, e.g. `"512MiB"`, `"2GiB"`.            |
| `cpus`   | int    | yes      | Number of virtual CPUs.                            |

The deploy spec must grant at least these values. Granting more is allowed.


### `[[requires.dataDirs]]`

Directories the application writes to at runtime. Each must be backed by a volume or host path in the deploy spec.

| Field  | Type   | Required | Description                                  |
|--------|--------|----------|----------------------------------------------|
| `path` | string | yes      | Absolute path inside the guest filesystem.   |

Example:

```toml
[[requires.dataDirs]]
path = "/var/lib/image-tool"

[[requires.dataDirs]]
path = "/var/cache/image-tool"
```

If the deploy spec does not provide a mount for every declared `dataDir`, the deployment is rejected.


### `[requires.network]`

Network access the package needs.

| Field      | Type    | Required | Description                                                |
|------------|---------|----------|------------------------------------------------------------|
| `outbound` | boolean | yes      | `false` means the package needs no network at all. `true` means it needs outbound network access. |

When `outbound = false`, the runtime attaches no network device to the VM. This is stronger than filtering: there is nothing to exploit.

When `outbound = true`, the deploy spec must set `network.mode = "nat"` or `"bridge"`. The deploy spec may further restrict the allowed destinations via `network.allow`.

If you need to declare *specific* destinations rather than all outbound traffic, set `outbound = true` and document the required destinations in your README. Per-destination requirements will be part of a future API version.


### `[requires.devices]`

Devices the package may use. All default to `false`.

| Field        | Type    | Required | Description              |
|--------------|---------|----------|--------------------------|
| `webcam`     | boolean | no       | Requires webcam access.  |
| `microphone` | boolean | no       | Requires microphone.     |
| `gpu`        | boolean | no       | Requires GPU access.     |

In v1, `gpu = true` is a validation error: GPU passthrough is a
non-goal.

If a device is `true`, the operator must explicitly grant it in the
deploy spec. The default deploy spec grants nothing.


### `[requires.display]`

Whether the package needs a display server.

| Field      | Type    | Required | Description                                    |
|------------|---------|----------|------------------------------------------------|
| `enabled`  | boolean | yes      | Whether the package needs a display.           |
| `protocol` | string  | no       | `"wayland"` or `"x11"`. Defaults to `"wayland"`. |

In v1, `enabled = true` is a validation error. GUI forwarding is a post-v1 feature.

### `[requires.audio]`

Whether the package needs audio output.

| Field     | Type    | Required | Description                         |
|-----------|---------|----------|-------------------------------------|
| `enabled` | boolean | yes      | Whether the package needs audio.    |

In v1, `enabled = true` is a validation error. Audio forwarding is a post-v1 feature.


## `[service]` *(optional)*

Present only if the package exposes a network service. Declaring a service does not grant network access, it tells the operator what the package will listen on inside its own guest.


### `[[service.ports]]`

| Field           | Type   | Required | Description                                        |
|-----------------|--------|----------|----------------------------------------------------|
| `name`          | string | yes      | Port name. Lowercase, DNS-safe.                    |
| `containerPort` | int    | yes      | Port inside the guest.                             |
| `protocol`      | string | yes      | `"tcp"` or `"udp"`.                                |


### `[service.health]`

Optional health check. VPOK uses this to decide when a deployment is ready and when to restart a failed instance.

| Field      | Type   | Required | Description                                          |
|------------|--------|----------|------------------------------------------------------|
| `type`     | string | yes      | `"http"` or `"tcp"`.                                 |
| `path`     | string | if http  | URL path for HTTP checks.                            |
| `port`     | int    | yes      | Port to check. Must match a declared service port.   |
| `interval` | string | no       | Check interval. Defaults to `"10s"`.                 |
| `timeout`  | string | no       | Per-check timeout. Defaults to `"2s"`.               |
| `failures` | int    | no       | Consecutive failures before restart. Defaults to `3`.|


### `[service.readiness]`

Optional readiness check. If absent, the health check is used.

| Field      | Type   | Required | Description                                       |
|------------|--------|----------|---------------------------------------------------|
| `type`     | string | yes      | `"http"` or `"tcp"`.                              |
| `path`     | string | if http  | URL path.                                         |
| `port`     | int    | yes      | Port to check.                                    |
| `interval` | string | no       | Defaults to `"5s"`.                               |


Example:

```toml
[service.health]
type = "http"
path = "/healthz"
port = 8080
interval = "10s"
timeout = "2s"
failures = 3

[[service.ports]]
name = "http"
containerPort = 8080
protocol = "tcp"
```

---


## `[reproducible]`

Reproducibility and trust metadata.

| Field     | Type    | Required | Description                                                      |
|-----------|---------|----------|------------------------------------------------------------------|
| `enabled` | boolean | no       | If `true`, `vpok build` verifies reproducibility by building twice and comparing digests. Defaults to `false`. |
| `sbom`    | boolean | no       | If `true`, emit a Software Bill of Materials alongside the package. Defaults to `false`. |

When `enabled = true`, the build must not embed timestamps, absolute paths or random IDs. If two builds of the same source produce different digests, the build fails.

---

## Validation rules

The build spec is valid if and only if:

1. `apiVersion` is `"vpok.io/v1"`.
2. `kind` is `"Build"`.
3. Every required field is present.
4. No unknown fields are present at any level.
5. `metadata.name` matches `[a-z0-9]([a-z0-9-]*[a-z0-9])?` and is at most 63 characters.
6. `metadata.version` is a valid semantic version.
7. `base.distribution` is a supported value.
8. `base.architecture` is a supported value.
9. Every entry in `build.steps` has a known `action`, and only the fields legal for that action.
10. Every `copy.from` is within the build context.
11. `entrypoint.command` is non-empty.
12. `requires.resources.memory` and `cpus` are present and positive.
13. Every `requires.dataDirs.path` is absolute.
14. `requires.devices.gpu` is not `true`.
15. `requires.display.enabled` is not `true`.
16. `requires.audio.enabled` is not `true`.
17. Every `service.ports.containerPort` is unique.
18. Every health or readiness check `port` matches a declared service port.

---

## Common patterns

### A CLI tool with no data directory

```toml
[metadata]
name = "jq"
version = "1.7.1"
publisher = "example.org"

[base]
distribution = "alpine"
release = "3.20"
architecture = "amd64"

[[build.steps]]
action = "run"
command = "apk add --no-cache jq"

[entrypoint]
command = ["/usr/bin/jq"]

[requires.resources]
memory = "32MiB"
cpus = 1
```

### A service with a persistent data directory

```toml
[[build.steps]]
action = "copy"
from = "./server"
to = "/opt/server/server"
mode = "0755"

[entrypoint]
command = ["/opt/server/server"]

[[requires.dataDirs]]
path = "/var/lib/server"

[requires.resources]
memory = "512MiB"
cpus = 2

[service.health]
type = "http"
path = "/healthz"
port = 8080

[[service.ports]]
name = "http"
containerPort = 8080
protocol = "tcp"
```

### A service that needs outbound network

```toml
[requires.network]
outbound = true
```

The operator must then set `network.mode = "nat"` or `"bridge"`
in the deploy spec. Document the destinations your package
contacts in your README so the operator can restrict them
appropriately.

---

## See also

- `architecture.md` — how the pieces fit together.
- `deploy-spec.md` — how operators run your package.
- `validation.md` — the full validation ruleset.
