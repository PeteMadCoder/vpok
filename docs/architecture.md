# VPOK Architecture

VPOK is a packaging and deployment system for isolated workloads. A VPOK package bundles an application or service together with the operating system environment it needs, and runs it inside a dedicated virtual machine.

## Goals

- Package applications and services as self-contained VM images.
- Provide hardware-level isolation between workloads and the host
- Make builds reproducible and artifacts content-addressed.
- Separate publisher intent (requirements) from operator policy (permissions).
- Function fully offline. The registry is a distribution channel, not a runtime dependency.
- Keep the priviledged attack surface as small as possible.

## 2. Non-goals for v1

- Multi-host scheduling and cluster orchestration.
- Live migration of running workloads.
- GPU passthrough.
- Non-Linux guests (Windows, macOS).
- A graphical package manager.

## System componenets
```mermaid
flowchart TB
    CLI["vpok CLI<br/><br/>build · publish · install · run · deploy"]

    API["local API (unix socket)"]

    subgraph HOST["vpokd · host daemon"]
        direction TB

        subgraph COMPONENTS[" "]
            direction LR
            STORE["Store<br/>(CAS)"]
            RUNTIME["Runtime<br/>backend"]
            REGISTRY["Registry<br/>client"]
        end
    end

    HYPERVISOR["KVM / hypervisor"]

    subgraph GUEST["Guest VM"]
        direction TB

        AGENT["vpok-agent<br/><br/>start · stop · report · mount data"]

        APP["Application / service process"]
    end

    CLI --> API
    API --> HOST

    HOST --> HYPERVISOR
    HYPERVISOR --> GUEST

    HOST --- STORE
    HOST --- RUNTIME
    HOST --- REGISTRY

    AGENT --> APP
```
