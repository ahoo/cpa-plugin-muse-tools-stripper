# cpa-plugin-muse-tools-stripper

CLIProxyAPI request interceptor for Codex clients on the opencode zen free
tier (`muse-spark-1.3-contributor-free`, aliases `muse-free` / `muse`).

Codex 0.15x sends `{"type": "additional_tools", ...}` as `input[0]` (exec
sandbox tool declarations). The zen free tier does not accept this input
type and rejects the whole request with:

```text
400 [invalid_request_error] `input[0]` did not match any supported type
```

Verified live: the same payload without the `additional_tools` entry
returns 200. This plugin drops `input[]` entries of type
`additional_tools` for free-tier models only, on the
`request.intercept_after` hook. Standard entries (`message`,
`function_call`, `function_call_output`, `reasoning`, custom tool calls)
pass through untouched.

Trade-off: the `exec` sandbox tool is unavailable behind this filter;
conversation, file edits and shell calls are unaffected.

Paid builds (`muse-spark-1.3-contributor` without a free marker) are
deliberately never filtered — they may support the type.

## Install

```yaml
plugins:
  enabled: true
  configs:
    muse-tools-stripper:
      enabled: true
      priority: 50
```

Restart and verify:

```bash
docker restart cli-proxy-api
docker logs cli-proxy-api | grep muse-tools-stripper
# pluginhost: plugin registered plugin_id=muse-tools-stripper ...
```

## Upstream note

Workaround for a backend capability gap, not a host bug. **Not submitted
to the official plugin store** — narrow, backend-specific, tracked for
removal if the zen tier accepts the type.

## Build

Debian/glibc toolchain only:

```bash
./build.sh
```

## Test

```bash
go vet ./... && go test ./...
```

## License

MIT
