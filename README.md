# reboot-agent

`reboot-agent` is a small Linux agent for unattended VPS restarts. It keeps no network listener and does not use cron, at, or systemd timers.

## What It Does

- Asks for a restart interval in hours during the first installation.
- Adds a fresh random delay of 1-59 minutes and 0-59 seconds to every cycle.
- Checks RAM plus swap every 5 seconds; the first-install memory threshold defaults to `80%`.
- Restarts immediately when memory usage is above the configured threshold.
- Uses the Linux reboot interface directly instead of executing a `reboot` command.
- Keeps the configuration after a reboot and starts a new cycle after boot.

There is intentionally no boot grace period or reboot-loop protection. If memory is still above the threshold after boot, the agent will try to restart again.

## Install

```sh
sh -c "$(curl -fsSL https://raw.githubusercontent.com/w243420707/reboot-agent/main/install.sh)"
```

The installer requires root, detects the Linux architecture, downloads the matching static binary, verifies SHA-256, asks for the hours and memory threshold, and configures one available boot mechanism. To install a different release, set `REBOOT_AGENT_VERSION`; to use a fork, set `REBOOT_AGENT_REPO`.

Supported release targets:

- `amd64`
- `arm64`
- `armv7`
- `386`
- `mips`
- `mipsle`
- `riscv64`

## Commands

```sh
reboot-agent configure
reboot-agent status
reboot-agent start
reboot-agent stop
reboot-agent uninstall
reboot-agent --version
```

`configure` asks for:

- Hours: `1-8760`, required.
- Memory threshold: `1-99` percent, or `0` to disable memory-triggered restarts.

The configuration is stored in `/etc/reboot-agent/config`. Runtime status is stored in `/run/reboot-agent/status`.

## Memory Detection

The agent first looks for the current process cgroup. cgroup v2 and cgroup v1 memory/swap limits are preferred so an LXC guest does not accidentally use the host's memory total. If a cgroup v1 environment does not expose swap accounting, the status output explicitly shows that it is using the RAM quota only. If no finite cgroup memory limit is available, it falls back to `/proc/meminfo`.

When using `/proc/meminfo`:

```text
used = (MemTotal - MemAvailable) + (SwapTotal - SwapFree)
total = MemTotal + SwapTotal
```

On older kernels without `MemAvailable`, the agent uses `MemFree + Buffers + Cached + SReclaimable - Shmem` as the available-RAM estimate.

If memory data cannot be read, the memory trigger is skipped and the time-based schedule continues.

## Startup Support

The installer tries one startup method in this order:

1. systemd service, without a timer.
2. OpenWrt `rc.common`/procd service.
3. OpenRC service.
4. SysV init script.
5. BusyBox `inittab` respawn entry.
6. `/etc/rc.local` fallback.

The agent must have permission to reboot the current Linux namespace. A VPS provider or LXC host may deny this even when the agent is running as root; in that case the failure is recorded in `status` and the agent remains alive.

## Build

```sh
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$(cat VERSION)" -o dist/reboot-agent-linux-amd64 .
```

GitHub Actions builds all release targets when a `v*` tag is pushed. Each binary and its SHA-256 file are uploaded to the GitHub Release.

## Changelog

### v0.1.3 - 2026-09-10

- Fix OpenRC automatic startup so it no longer conflicts with the Agent's own process record.
- Keep the Agent enabled when OpenRC or SysV stops it during a normal system shutdown.
- Preserve existing configuration when rerunning the installer for an update.

### v0.1.2 - 2026-09-10

- Correct the published release-date metadata and update the one-command installer to the latest release.

### v0.1.1 - 2026-09-10

- Set the default GitHub repository for the one-command installer.
- Publish the initial public GitHub release configuration.

### v0.1.0 - 2026-09-10

- Initial Linux agent implementation.
- Hour-based restart schedule with random minute and second offsets.
- RAM plus swap threshold monitoring.
- cgroup v1/v2-aware memory detection.
- Compatibility fallback for kernels without `MemAvailable`.
- Direct Linux reboot request.
- systemd, OpenWrt, OpenRC, SysV, BusyBox, and `rc.local` startup support.
- Architecture-aware GitHub Release installer.
