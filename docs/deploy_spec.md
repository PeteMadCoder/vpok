# VPOK Deployment Specification

The deploy spec describes how a package runs on a specific host. It is authored by the operator. It is never signed by the published and never travels with the package.

**Audience:** operators running VPOK packages on their hosts.
**Default filename:** `vpok.deploy.toml`.
**Format:** TOML.
**API Version:** `vpok.io/v1`.

A single host may hold many deploy specs, one per instance (or mode). Deploy specs are typically kept as separate files:

```
deploy/alice-laptol.vpok.deploy.toml
deploy/prod-eu-1.vpok.deploy.toml
```

and passed explicitly:

```bash
vpok deploy -f deploy/prod-eu-1.vpok.deploy.toml
```

---

## Minimal example

```toml
apiVersion = "vpok.io/v1"
kind = "Deploy"

[metadata]
name = "hello"

[package]
source = "registry.vpok.io/example/hello"
version = "0.1.0"

[resources]
memory = "64MiB"
cpus = 1
```

---

## Complete example

```toml
apiVersion = "vpok.io/v1"
kind = "Deploy"

# ---------------------------------------------------------------
# Identity
# ---------------------------------------------------------------

[metadata]
name = "image-tool"
description = "Alice's image tool"

# ---------------------------------------------------------------
# What to run
# ---------------------------------------------------------------

[package]
source = "registry.vpok.io/example/image-tool"
version = "1.4.0"

[package.verify]
signature = "required"
publisher = "example.org"

# ---------------------------------------------------------------
# Resources GRANTED
# ---------------------------------------------------------------

[resources]
memory = "1GiB"
cpus = 2
pids = 256
disk = "4GiB"

# ---------------------------------------------------------------
# Storage
# ---------------------------------------------------------------

[[storage]]
name = "data"
guestPath = "/var/lib/image-tool"
type = "volume"
size = "2GiB"
persistent = true

[[storage]]
name = "pictures"
guestPath = "/pictures"
type = "hostPath"
hostPath = "$HOME/Pictures"
mode = "ro"

[[storage]]
name = "output"
guestPath = "/output"
type = "hostPath"
hostPath = "$HOME/Pictures/Processed"
mode = "rw"

# ---------------------------------------------------------------
# Network
# ---------------------------------------------------------------

[network]
mode = "none"

# ---------------------------------------------------------------
# Devices
# ---------------------------------------------------------------

[devices]
webcam = false
microphone = false
gpu = false

# ---------------------------------------------------------------
# Display and audio
# ---------------------------------------------------------------

[display]
enabled = false

[audio]
enabled = false

# ---------------------------------------------------------------
# Secrets
# ---------------------------------------------------------------

[[secrets]]
name = "api-key"
source = "env:IMAGE_TOOL_API_KEY"

# ---------------------------------------------------------------
# Environment
# ---------------------------------------------------------------

[env]
LOG_LEVEL = "info"
TZ = "Europe/Lisbon"

# ---------------------------------------------------------------
# Lifecycle
# ---------------------------------------------------------------

[restart]
policy = "on-failure"
maxAttempts = 3
backoff = "5s"

[shutdown]
gracePeriod = "10s"
```

---

## Top-level fields

| Field        | Type   | Required | Description                                |
|--------------|--------|----------|--------------------------------------------|
| `apiVersion` | string | yes      | Always `"vpok.io/v1"` for this version.    |
| `kind`       | string | yes      | Always `"Deploy"`.                         |

Unknown top-level keys are rejected.

---

## `[metadata]`

Identifies this deployment instance on the local host.

| Field         | Type   | Required | Description                                                     |
|---------------|--------|----------|-----------------------------------------------------------------|
| `name`        | string | yes      | Instance name. Lowercase, DNS-safe. Must be unique per host.    |
| `description` | string | no       | Human-readable description.                                     |

The instance name is local. It is not sent to a registry and is not part of the package identity.

---

## `[package]`

Which package to run, and where to get it.

| Field     | Type   | Required | Description                                                       |
|-----------|--------|----------|-------------------------------------------------------------------|
| `source`  | string | yes      | Registry reference, e.g. `"registry.vpok.io/example/image-tool"`. |
| `version` | string | no       | Version to install. Ignored if `digest` is set.                   |
| `digest`  | string | no       | Exact manifest digest, e.g. `"sha256:..."`. Pins the package.     |

If both `version` and `digest` are set, `digest` wins. Use `digest` for production deployments, it removes ambiguity about what is being run.

If neither `version` not `digest` is set, the deployment fails. There is no implicit `latest`.


### `[package.verify]`

Signature verification policy.

| Field       | Type   | Required | Description                                                              |
|-------------|--------|----------|--------------------------------------------------------------------------|
| `signature` | string | no       | `"required"`, `"preferred"`, or `"off"`. Defaults to `"preferred"`.      |
| `publisher` | string | no       | Expected publisher identity. If set, the manifest must be signed by this publisher. |

- `"required"` - refuse to install or run a package without a valid signature.
- `"prefered"` - verify if a signature is present, warn if it is not.
- `"off"` - do not verify signatures. Not recommended outside of development.

---

## `[resources]`

Resources **granted** to this deployment. These must be at least as large as the package's declared `requires.resources`.

| Field    | Type   | Required | Description                                                |
|----------|--------|----------|------------------------------------------------------------|
| `memory` | string | yes      | Memory granted, e.g. `"1GiB"`.                             |
| `cpus`   | int    | yes      | vCPUs granted.                                             |
| `pids`   | int    | no       | Maximum number of processes. Defaults to `256`.            |
| `disk`   | string | no       | Maximum writable disk. Defaults to `4GiB`.                 |

If `memory` or `cpus` is smaller than the package requires, the deployment is rejected with an error naming the required value.

---

## `[[storage]]`

Mounts inside the guest. Every path declared in the package's `requires.dataDirs` must appear here with a matching `guestPath`.

Each entry is a **tagged union**: `type` selects which other fields are legal.


### `type = "volume"`

A VPOK-managed volume. Backed by a sparse file on the host managed by `vpokd`.

| Field        | Type    | Required | Description                                          |
|--------------|---------|----------|------------------------------------------------------|
| `name`       | string  | yes      | Volume name. Unique within the deployment.           |
| `guestPath`  | string  | yes      | Absolute mount path inside the guest.                |
| `type`       | string  | yes      | `"volume"`.                                          |
| `size`       | string  | yes      | Volume size, e.g. `"2GiB"`.                          |
| `persistent` | boolean | no       | If `true`, the volume survives deployment removal. Defaults to `false`. |

Persistent volumes are stored under `/var/lib/vpok/volumes/<deployment>/<name>`. Removing a deployment with `vpok destroy` does not delete persistent volumes unless `--purge` is passed.


### `type = "hostPath"`

A host directory shared into the guest via virtio-fs.

| Field       | Type   | Required | Description                                                          |
|-------------|--------|----------|----------------------------------------------------------------------|
| `name`      | string | yes      | Mount name. Unique within the deployment.                            |
| `guestPath` | string | yes      | Absolute mount path inside the guest.                                |
| `type`      | string | yes      | `"hostPath"`.                                                        |
| `hostPath`  | string | yes      | Host path. `$HOME`, `$XDG_DATA_HOME`, and `$XDG_CONFIG_HOME` are expanded by `vpokd` at start time. |
| `mode`      | string | yes      | `"ro"` or `"rw"`.                                                    |

`hostPath` must be an absolute path after expansion. Paths outside the operator's home directory require an explicit `--allow-host-path` flag when running `vpok deploy`.

Host paths are shared with the guest through a narrow virtio-fs mount. The host filesystem is not otherwise exposed.

Example:

```toml
[[storage]]
name = "pictures"
guestPath = "/pictures"
type = "hostPath"
hostPath = "$HOME/Pictures"
mode = "ro"
```

---


## `[network]`

Network policy. The default is no network at all.

| Field  | Type   | Required | Description                                                       |
|--------|--------|----------|-------------------------------------------------------------------|
| `mode` | string | yes      | `"none"`, `"nat"`, or `"bridge"`.                                 |
| `dns`  | array<string> | no | DNS servers to use inside the guest. Only meaningful with `nat` or `bridge`. |

- `"none"` - no network device is attached. This is the safest mode.
- `"nat"` - the VM gets a private IP and can reach destinations permitted by `allow`, subject to NAT.
- `"bridge"` - the VM is attached to a host bridge and gets an address on the bridge network. Use with care.

If the package declares `requires.network.outbound = false`, the only legal value is `"none"`. Setting anything else is a validation error.

If the package declares `requires.network.outbound = true`, the mode must be `"nat"` or `"bridge"`. The `allow` list may further restrict the destinations.


### `[[network.allow]]`

Optional allowlist. Each entry is either a host or a CIDR, plus a port.

| Field  | Type   | Required | Description                                                       |
|--------|--------|----------|-------------------------------------------------------------------|
| `host` | string | one of   | Exact hostname.                                                   |
| `cidr` | string | one of   | CIDR block.                                                       |
| `port` | int    | yes      | Port.                                                             |

Exactly one of `host` and `cidr` must be set. If `allow` is empty or absent, all outbound traffic is allowed in `nat` mode. To deny all traffic, use `mode = "none"`.

Example:

```toml
[network]
mode = "nat"
dns = ["1.1.1.1"]

[[network.allow]]
host = "updates.example.org"
port = 443

[[network.allow]]
cidr = "10.0.0.0/8"
port = 5432
```

## `[devices]`

Device grants. All defaults to `false`.

| Field        | Type    | Required | Description                                    |
|--------------|---------|----------|------------------------------------------------|
| `webcam`     | boolean | no       | Grant webcam access.                           |
| `microphone` | boolean | no       | Grant microphone access.                       |
| `gpu`        | boolean | no       | Grant GPU access. Not supported in v1.         |

A device may be granted only if the package declares it in `requires.devices`. Granting a device the package does not need is a validation error.

---

## `[display]`

Display grant.

| Field      | Type    | Required | Description                                            |
|------------|---------|----------|--------------------------------------------------------|
| `enabled`  | boolean | yes      | Whether to enable display forwarding. Defaults to `false`. |
| `protocol` | string  | no       | `"wayland"` or `"x11"`. Defaults to `"wayland"`.       |

Display forwarding is not supported in v1. `enabled = true` is a
validation error.

---

## `[audio]`

Audio grant.

| Field     | Type    | Required | Description                                        |
|-----------|---------|----------|----------------------------------------------------|
| `enabled` | boolean | yes      | Whether to enable audio forwarding. Defaults to `false`. |

Audio forwarding is not supported in v1. `enabled = true` is a validation error.

---

## `[secrets]`

Secrets injected into the guest at start time. Secrets are resolved by `vpokd` and delivered to `vpok-agent` over vsock. They are never written to disk in the local store and never appear in the manifest.

| Field    | Type   | Required | Description                                                       |
|----------|--------|----------|-------------------------------------------------------------------|
| `name`   | string | yes      | Secret name. Exposed as a file under `/run/vpok/secrets/<name>` inside the guest. |
| `source` | string | yes      | Where to read the value. See below.                               |

Supported sources:
| Source                | Meaning                                                     |
|-----------------------|-------------------------------------------------------------|
| `env:NAME`            | Read from the environment variable `NAME` on the host.      |
| `file:/absolute/path` | Read from a file on the host. The value is the file contents. |


Example:

```toml
[[secrets]]
name = "api-key"
source = "env:IMAGE_TOOL_API_KEY"

[[secrets]]
name = "tls-cert"
source = "file:/etc/vpok/secrets/tls.pem"
```

Inside the guest, each secret is available as a file under `/run/vpok/secrets`. The file is owned by root, mode "0400". The entrypoint runs as root unless the package specifies otherwise.

The `vpok.deploy.toml` file contains no secret values, only references. This makes it safe to commit to version control.

---

## `[env]`

Environment variables passed to the entrypoint inside the guest.

| Field | Type   | Required | Description                                     |
|-------|--------|----------|-------------------------------------------------|
| *     | string | no       | Plain key/value pairs. Keys and values are strings. |

Example:

```toml
[env]
LOG_LEVEL = "info"
TZ = "Europe/Lisbon"
```

Environment variables are not secret. Use `[[secrets]]` for values that should not appear in process listings or logs.

---

## `[restart]`

Restart policy.

| Field         | Type   | Required | Description                                                       |
|---------------|--------|----------|-------------------------------------------------------------------|
| `policy`      | string | no       | `"never"`, `"on-failure"`, or `"always"`. Defaults to `"on-failure"`. |
| `maxAttempts` | int    | no       | Maximum consecutive restart attempts. Defaults to `3`.            |
| `backoff`     | string | no       | Delay between restart attempts. Defaults to `"5s"`.

- `"never"` - the deployment runs once. If it exits, it stays stopped.
- `"on-failure"` - restart if the process exits with a nonzero status.
- `"always"` - restart regardless of exit status.

`maxAttempts` is reset after a successfull run of at least 10 seconds.

---

## `[shutdown]`

Shutdown behavior.

| Field         | Type   | Required | Description                                                       |
|---------------|--------|----------|-------------------------------------------------------------------|
| `gracePeriod` | string | no       | How long to wait after `SIGTERM` before `SIGKILL`. Defaults to `"10s"`. |

The `SIGTERM` is sent to the entrypoint by `vpok-agent`. If the process does not exit within `gracePeriod`, it is killed.

---

## `[service]` *(optional)*

Present only if the package declares a service.


### `[[service.publish]]`

Expose a service port on the host

| Field      | Type   | Required | Description                                          |
|------------|--------|----------|------------------------------------------------------|
| `name`     | string | yes      | Must match a `name` in the package's `service.ports`.|
| `hostPort` | int    | yes      | Host port to bind.                                   |


### `[service.update]`

Update behavior.

| Field               | Type    | Required | Description                                                       |
|---------------------|---------|----------|-------------------------------------------------------------------|
| `strategy`          | string  | no       | `"recreate"` or `"rolling"`. Defaults to `"recreate"`.            |
| `rollbackOnFailure` | boolean | no       | If `true`, automatically roll back to the previous version if the new version fails health checks. Defaults to `true`. |

`"rolling"` is accepted but behaves as `"recreate"` in v1. Multi-VM rolling updates are a post-v1 feature.

---

## `[licence]` *optional*

For commercial packages.

| Field        | Type   | Required | Description                                                      |
|--------------|--------|----------|------------------------------------------------------------------|
| `document`   | string | yes      | Path to a signed license document.                               |
| `activation` | string | no       | `"offline"` or `"online"`. Defaults to `"offline"`.              |
| `server`     | string | no       | License server URL. Required if `activation = "online"`.         |

If the package requires a license and no license is configured, the deployment fails at start time.

---


## Validation rules

The deploy spec is valid if and only if:

1. `apiVersion` is `"vpok.io/v1"`.
2. `kind` is `"Deploy"`.
3. Every required field is present.
4. No unknown fields are present at any level.
5. `metadata.name` matches `[a-z0-9]([a-z0-9-]*[a-z0-9])?` and is at most 63 characters.
6. `metadata.name` is unique across deployments on this host.
7. `package.source` is a valid registry reference.
8. At least one of `package.version` and `package.digest` is set.
9. For every `path` in the manifest's `requires.dataDirs`, there is a `[[storage]]` entry with `guestPath == path`.
10. `resources.memory` >= `manifest.requires.resources.memory`.
11. `resources.cpus` >= `manifest.requires.resources.cpus`.
12. If `manifest.requires.network.outbound == false`, `network.mode` must be `"none"`.
13. If `manifest.requires.network.outbound == true`, `network.mode` must be `"nat"` or `"bridge"`.
14. Every `[[network.allow]]` entry has exactly one of `host` and `cidr`.
15. Every `[[devices]]` grant is also declared in `manifest.requires.devices`.
16. `display.enabled` must be `false` unless `manifest.requires.display.enabled` is `true`.
17. `audio.enabled` must be `false` unless `manifest.requires.audio.enabled` is `true`.
18. Every `[[storage]]` entry has a unique `name`.
19. Every `[[storage]]` entry has a unique `guestPath`.
20. `storage.hostPath` must not contain `..` segments after expansion.
21. Every `[[secrets]]` entry has a unique `name`.
22. If `package.verify.signature == "required"`, the manifest must carry a valid signature.
23. If `package.verify.publisher` is set, the signature must be from that publisher.
24. If the manifest declares a service, `service.publish` must be present with a matching `name` for each declared port.

Additional runtime rules apply but are not checked at validation time (for example, disk quota enforcement).

---


## Common patterns

### Run a CLI tool with no persistent state

```toml
[metadata]
name = "jq"

[package]
source = "registry.vpok.io/example/jq"
version = "1.7.1"

[resources]
memory = "32MiB"
cpus = 1

[network]
mode = "none"
```

### Run a service with persistent data, no network

```toml
[metadata]
name = "notes"

[package]
source = "registry.vpok.io/example/notes"
version = "2.0.0"

[resources]
memory = "512MiB"
cpus = 2

[[storage]]
name = "data"
guestPath = "/var/lib/notes"
type = "volume"
size = "1GiB"
persistent = true

[network]
mode = "none"
```

### Run a service with restricted outbound network

```toml
[network]
mode = "nat"

[[network.allow]]
host = "api.stripe.com"
port = 443

[[network.allow]]
host = "smtp.example.org"
port = 587
```

### Share a host directory read-only

```toml
[[storage]]
name = "documents"
guestPath = "/documents"
type = "hostPath"
hostPath = "$HOME/Documents"
mode = "ro"
```

### Inject a secret from the host environment

```toml
[[secrets]]
name = "database-url"
source = "env:DATABASE_URL"
```

---

## Operational notes

### Where deployments live

`vpokd` stores deployment state under `/var/lib/vpok/deployments/<name>/`:
- `spec.toml` — a copy of the deploy spec at deploy time.
- `resolved.json` — the resolved manifest digest and layer digests.
- `state.json` — current status, restart count, last exit code.
- `volumes/` — persistent volumes belonging to this deployment.


### Removing a deployment

```bash
vpok destroy notes
```

This stops the VM and removes the deployment state. Persistent volumes are preserved. To delete them too:

```bash
vpok destroy notes --purge
```


### Updating a deployment

```bash
vpok update notes -f deploy/notes.vpok.deploy.toml
```

The deploy spec is revalidated against the new package version. If the new version requires grants the deploy spec does not provide, the update is rejected and the previous version keeps running.


### Offline operation

Once a package is installed, no network access is required to run it. The registry is only contacted for `install`, `update`, and `publish`.

---

## See also

- `architecture.md` — how the pieces fit together.
- `build_spec.md` — how publishers build packages.
- `validation.md` — the full validation ruleset.
