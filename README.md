# cpa-plugin-muse-tools-stripper

A narrowly scoped CLIProxyAPI compatibility plugin for Codex Responses traffic sent to OpenCode Zen Muse free-tier models (`muse-spark-1.3-contributor-free`, including the `muse-free` and `muse` aliases).

Codex clients can send an `input[]` entry shaped like this:

```json
{"type": "additional_tools", "tools": []}
```

The Muse free tier currently rejects that entry with:

```text
400 [invalid_request_error] `input[0]` did not match any supported type
```

The plugin removes only `input[]` entries whose type is `additional_tools`. It preserves messages, function calls and outputs, reasoning items, custom tool calls, and every other input type.

## Scope and trade-off

Filtering is intentionally fail-closed by target identity and fail-open by payload:

- Muse/OpenCode Zen aliases with a `-free` identity are eligible.
- Paid Muse contributor models without the free marker are never changed.
- Unrelated models and traffic targeting non-Codex formats are never changed.
- Missing, malformed, or non-Responses inner payloads pass through unchanged.
- Thinking suffixes such as `muse-free(high)` and provider-prefixed aliases are recognized.
- Running both interception and normalization is idempotent.

The filtered `additional_tools` declaration represents the Codex exec-sandbox capability, so that capability is unavailable behind this workaround. Conversation, ordinary function tools, file edits, and shell calls represented by supported input types are unaffected.

## Hooks

The plugin registers both request-interceptor and request-normalizer capabilities:

1. `request.intercept_before` and `request.intercept_after` handle requests that already contain a Codex `input[]` payload before or after credential selection.
2. `request.normalize` handles provider payloads after CLIProxyAPI translation when the translator itself generated `input[]`.

Using all three hooks covers the supported request paths without modifying CLIProxyAPI itself.

## Install

After the plugin is accepted into the official CLIProxyAPI Plugins Store, install `muse-tools-stripper` through the host's plugin-management UI. For a manual installation, download the archive for the host platform from the matching GitHub Release, verify it against `checksums.txt`, extract the single library at the plugin-directory root, enable the plugin, and restart CLIProxyAPI so the Go shared library is loaded.

Example configuration:

```yaml
plugins:
  enabled: true
  configs:
    muse-tools-stripper:
      enabled: true
      priority: 50
```

A successful startup registers both capabilities under the exact ID `muse-tools-stripper`.

## Build

The local build uses a pinned Debian/glibc Go image, mounts the source read-only, runs formatting/module/vet/test/race gates, and writes only to repository-local staging by default:

```bash
./build.sh
# dist/local/linux_amd64/muse-tools-stripper-v0.1.3.so
```

Override the staging directory with `PLUGIN_OUT_DIR`. Do not point it at a live plugin mount; validate the staged artifact before deployment.

For direct source checks:

```bash
test -z "$(gofmt -l ./*.go ./.github/scripts/*.go)"
go mod verify
go vet ./...
go vet ./.github/scripts
go test ./...
go test ./.github/scripts
go test -race ./...
```

## Releases

Tags build five native archives:

```text
muse-tools-stripper_<version>_<goos>_<goarch>.zip
```

Each ZIP has exactly one canonical root library (`muse-tools-stripper.so`, `.dylib`, or `.dll`), fixed metadata for reproducibility, and an entry in `checksums.txt`. The workflow refuses to overwrite an existing GitHub Release.

A distinct local binary already used version `0.1.2` before release hardening, so the hardened immutable release is `v0.1.3`; no published version is reused or replaced.

## Removal

This is a backend capability workaround. Remove or disable the plugin once the Muse free tier accepts `additional_tools` natively, after validating equivalent requests without the filter.

## License

MIT
