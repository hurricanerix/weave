# Weave Security Architecture

This document describes the security model for the Weave daemon, a C application that manages GPU resources for generative AI diffusion models. The daemon communicates with client applications (primarily a Go web UI) via a Unix domain socket.

## Design Principles

- **Zero trust**: Assume any connecting process could be compromised
- **Defense in depth**: Multiple layers of security, each providing independent protection
- **Security from the start**: Security architecture designed before implementation, not retrofitted

## Architecture Overview

```
┌────────────────────────┐         ┌─────────────────────────────┐
│ Weave Application (Go) │         │   weave-compute (C Daemon)  │
│      (user-facing)     │◄───────►│          (GPU work)         │
│                        │  Unix   │                             │
│     Network-exposed    │  Socket │          No network         │
└────────────────────────┘         └─────────────────────────────┘
```

The Weave application is the network-facing component and represents the primary attack surface. If compromised, an attacker would gain access to the Unix socket. The weave-compute's security model accounts for this.

## Deployment Modes

### Userland Mode
- weave-compute runs as the installing user
- Config location: `~/.config/weave/`
- Socket location: `$XDG_RUNTIME_DIR/weave/weave.sock`
- Trust model: Only processes running as the same UID can connect

### System Daemon Mode
- weave-compute daemon runs as a dedicated `weave` user or root
- Config location: `/etc/weave/`
- Socket location: `/run/weave/weave.sock`
- Trust model: Processes in an allowed group (e.g., `weave`) can connect
- Config files must be owned by root or the daemon user, not writable by regular users

## Security Layers

### Layer 1: Socket Authentication (MVP)

**Unix Socket Peer Credentials (`SO_PEERCRED`)**

On every incoming connection, immediately after `accept()`, the daemon retrieves the connecting process's credentials using `getsockopt()` with `SO_PEERCRED`. This provides the UID, GID, and PID of the connecting process, guaranteed by the kernel and unforgeable.

Implementation requirements:
- Call `getsockopt(fd, SOL_SOCKET, SO_PEERCRED, &cred, &len)` before reading any data
- In userland mode: reject connections where `cred.uid != getuid()`
- In system mode: verify connecting UID/GID against allowed list
- On rejection: close the socket immediately without sending any response

**Socket File Permissions**

- Create socket with mode `0660` (owner and group read/write) or `0600` (owner only)
- In system mode, set group ownership to the allowed group

### Layer 2: Seccomp-BPF Syscall Filtering (Post-MVP)

After initialization is complete, the daemon will apply a seccomp-BPF filter to restrict available syscalls to the minimum required set:

- Socket operations: `accept`, `read`, `write`, `close`, `getsockopt`
- File operations: `open`, `read`, `close`, `fstat` (for model loading)
- Memory operations: `mmap`, `munmap`, `brk`
- GPU operations: relevant `ioctl` calls for GPU driver
- Process basics: `exit`, `exit_group`, `sigreturn`

This limits the blast radius if an attacker finds a vulnerability in the daemon or underlying libraries.

**Implementation note**: Structure code with a clear "initialization complete" boundary where the seccomp filter will be applied. Even before implementation, mark this location in code.

### Layer 3: Mandatory Access Control (Post-MVP)

**AppArmor Profile (Ubuntu/Debian)**

Ship an AppArmor profile with the package that restricts:
- File read access: config directory, model directories only
- File write access: log files, socket only
- Network access: denied (daemon has no legitimate network needs)
- Capabilities: deny all

**SELinux Policy (RHEL/Fedora)**

Equivalent SELinux policy for Red Hat-based distributions.

### Layer 4: Input Validation (MVP)

**Model Selection by ID**

Models and their filesystem paths are defined in configuration at launch time. Clients request models by ID, never by path. The daemon performs a lookup and rejects unknown IDs.

This design eliminates path traversal vulnerabilities entirely.

**Parameter Bounds Checking**

All generation parameters must be validated:
- Image dimensions: minimum and maximum width/height
- Step count: reasonable upper bound
- CFG scale: valid range
- Seed: any value acceptable (unsigned integer)
- Other model-specific parameters: defined per model type

Reject requests with out-of-bounds parameters before any processing.

**Rate Limiting / Queue Management**

Consider implementing:
- Maximum queue depth per client
- Rate limiting on requests
- Timeout for queued requests

This prevents resource exhaustion attacks.

## API Surface

The daemon exposes a minimal API:

| Operation | Description | Validation |
|-----------|-------------|------------|
| `load_model(model_id)` | Load a model into VRAM | Must be known ID from config |
| `generate(params)` | Generate image with current model | All params bounds-checked |
| `unload()` | Explicitly unload current model | None required |
| `status()` | Return daemon/model status | None required |
| `list_models()` | Return available model IDs | None required |

Models are automatically unloaded after a configurable idle timeout to prevent VRAM exhaustion while avoiding unnecessary load/unload cycles during active use.

## Configuration Security

| Mode | Config Location | Required Ownership | Required Permissions |
|------|-----------------|-------------------|---------------------|
| Userland | `~/.config/weave/` | User | `0600` or `0644` |
| System | `/etc/weave/` | root or daemon user | `0644` (not world-writable) |

In system mode, config file integrity is critical — a writable config allows an attacker to add malicious model paths.

## Threat Model

| Threat | Likelihood | Mitigation |
|--------|------------|------------|
| Malicious local users | Low (home users) | `SO_PEERCRED` + socket permissions |
| Compromised Go app | Medium | Per-request validation, minimal API |
| Malicious model files | Medium | Paths from config only, not client input |
| Resource exhaustion | Medium | Queue limits, idle timeout, param bounds |
| Daemon code vulnerability | Low | Seccomp filter, AppArmor/SELinux |

## Implementation Checklist

### MVP (Layer 1 + Layer 4)
- [ ] `SO_PEERCRED` check immediately after `accept()`, before reading any data
- [ ] Silent close on UID mismatch (no response to unauthorized clients)
- [ ] Socket created with 0600 permissions (owner only for userland mode)
- [ ] Model lookup by ID only (paths loaded from config at startup)
- [ ] Parameter bounds checking on all generation requests
- [ ] Clear "initialization complete" code boundary (prep for seccomp)

### Post-MVP (Layer 2 + Layer 3)
- [ ] Seccomp-BPF filter applied after initialization
- [ ] AppArmor profile shipped with .deb package
- [ ] SELinux policy shipped with .rpm package
- [ ] System daemon mode with GID-based authorization
- [ ] Rate limiting / queue depth enforcement
- [ ] Audit logging of rejected connections and invalid requests

## References

- `man 7 unix` — Unix domain socket documentation
- `man 2 getsockopt` — SO_PEERCRED usage
- `man 2 seccomp` — Seccomp-BPF filtering
- [Kernel seccomp documentation](https://www.kernel.org/doc/html/latest/userspace-api/seccomp_filter.html)
- [AppArmor documentation](https://wiki.archlinux.org/title/AppArmor)
