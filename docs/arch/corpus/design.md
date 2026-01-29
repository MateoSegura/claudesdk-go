# Corpus Design

## Philosophy

A corpus is a curated set of **real, historical build failures** from upstream projects. Each entry pins a specific broken commit and defines an automated evaluation to verify whether a fix restores the build. The goal is to benchmark AI agents on their ability to diagnose and resolve real-world embedded build issues — not synthetic puzzles.

Key principles:

1. **Real bugs only** — every entry comes from a merged upstream issue with a known fix PR.
2. **Exact reproducibility** — broken commits are pinned by SHA so results are deterministic.
3. **Automated evaluation** — success/failure is determined by an exit code, not human judgment.
4. **Containerized isolation** — each entry runs in a Docker container with all toolchains pre-installed.

## Issue Selection Criteria

To be included in a corpus, an issue must satisfy:

| Criterion | Rationale |
|-----------|-----------|
| Public upstream issue with a merged fix PR | Ensures ground truth exists |
| Build failure (not runtime) | Evaluation via `west build` exit code |
| Reproducible from a single commit checkout | No multi-repo coordination needed |
| Fix is self-contained (1-3 files changed) | Keeps difficulty bounded |
| Affects a supported board/SoC (e.g., ESP32) | Must work with available toolchains |
| Does not require hardware-in-the-loop | Container-only evaluation |

### Difficulty Levels

- **easy** — Missing include, typo, simple rename. Fix is obvious from the error message.
- **medium** — Requires understanding build system config (Kconfig, DTS, linker scripts) or cross-module dependencies.
- **hard** — Requires architectural understanding (memory layout, boot sequences, HAL integration).

## Evaluation Strategy

Each corpus entry specifies:

```yaml
evaluation:
  command: "west build -b <board> <app_path>"
  success_exit_code: 0
```

The evaluation runs inside the container after the agent has made its changes. A zero exit code means the build succeeded (fix is correct). Any non-zero exit code means the build still fails.

For entries that require module updates after checkout:

```yaml
setup_commands:
  - ". /root/zephyrproject/zephyr/zephyr-env.sh"
  - "cd /root/zephyrproject/zephyr && git checkout <broken_sha>"
  - "west update"
```

## Docker Image Requirements

Corpus entries reference a container image that must provide:

| Component | Path / Details |
|-----------|---------------|
| Zephyr source tree | `/root/zephyrproject/zephyr/` (full clone with modules) |
| Zephyr SDK toolchains | `/usr/local/zephyr-sdk/` (named volume mount) |
| Build tools | `west`, `cmake`, `ninja`, `dtc` |
| Target toolchains | ESP32 (xtensa-esp32), ARM (arm-zephyr-eabi), etc. |
| Python venv | Zephyr's pip requirements pre-installed |

The SDK toolchain directory uses a **named Docker volume** (`zephyr-sdk`) so it persists across container runs and doesn't need to be re-downloaded.

## Corpus File Format

Corpus files are YAML with this structure:

```yaml
name: "corpus-name"
description: "Human-readable description"
image: "ghcr.io/org/image:tag"
volumes:
  volume-name: /mount/path

entries:
  - id: "unique-id"
    issue: "https://github.com/org/repo/issues/123"
    fix_pr: "https://github.com/org/repo/pull/124"
    broken_sha: "abc1234"
    difficulty: "easy"
    description: "What is broken and why"
    board: "esp32_devkitc_wroom"
    app_path: "samples/some/app"
    setup_commands:
      - "command 1"
      - "command 2"
    evaluation:
      command: "west build -b esp32_devkitc_wroom samples/some/app"
      success_exit_code: 0
```

## How to Add Entries

1. Find a Zephyr issue with a **build failure** and a **merged fix PR**.
2. Identify the broken commit SHA (last commit before the fix was applied).
3. Verify reproducibility: checkout the SHA, run `west build`, confirm it fails.
4. Identify the board and sample app that demonstrates the failure.
5. Add the entry to the appropriate corpus YAML file.
6. Test with `--dry-run` to validate the entry loads correctly.

## Limitations

- **Build failures only** — runtime bugs, test failures, and flaky issues are out of scope.
- **Single-board evaluation** — each entry targets one board. The same fix may or may not apply to other boards.
- **Network dependency** — `git checkout` and `west update` require network access inside the container.
- **Module version coupling** — some fixes require specific module versions; `west update` after checkout may pull incompatible versions.
- **No partial credit** — evaluation is binary (build passes or fails). A fix that resolves 3 of 4 errors still fails.
