# Reverse Engineering MCP Extension for PicoClaw Extended (Unified Spec)

## Project Context

- Repo: `https://github.com/souravrouth5/picoclaw-extended`, branch: `development`
- Binary name: `picoclawx`
- Config file: `~/.picoclaw/config.json`
- Already has embedded assets for workspace templates in `cmd/picoclaw/internal/onboard/` using Go embed FS — same pattern to be used for MCP assets
- MCP config structure already exists in `pkg/config/config.go` under `tools.mcp.servers` — each server has `enabled`, `command`, `args`, `type` fields
- `MCPServerConfig` and `MCPConfig` structs already defined, no changes needed there

---

## Directory Structure

```
mcp/
  re/
    SPEC.md
    re-static.py
    re-device.py
    re-dynamic.py
    scripts/
      ssl-unpin.js
      root-bypass.js
      method-trace.js
      crypto-hooks.js
      network-intercept.js
      intent-monitor.js
      shared-prefs.js
    install/
      termux.sh
      linux.sh
      macos.sh
      windows.bat

cmd/picoclaw/internal/mcp/
  manager.go
  detect.go
  embed.go

cmd/picoclaw/internal/agent/          ← NEW
  planner.go                          ← NEW: intent → structured plan
  executor.go                         ← NEW: step-by-step plan execution
  evaluator.go                        ← NEW: validate output, detect failure, decide next step
  memory.go                           ← NEW (optional): persistent analysis memory
  workflows.go                        ← NEW: predefined workflow definitions
  llm_interpreter.go                  ← NEW: LLM call to interpret raw findings → insights

cmd/picoclaw/
  mcp_cmd.go
```

---

## CLI Commands

```
picoclawx mcp list                   # show available servers + enabled status
picoclawx mcp enable re-static       # extract + dep check + configure
picoclawx mcp enable re-device
picoclawx mcp enable re-dynamic
picoclawx mcp disable re-static      # remove from config
picoclawx mcp deps re-static         # show deps without enabling
picoclawx mcp install-deps re-static # run the install script
```

### `enable` Flow

1. Extract `.py` server file to `~/.picoclaw/mcp/re/`
2. Extract Frida JS scripts to `~/.picoclaw/mcp/re/scripts/` (re-dynamic only)
3. Detect platform (Termux / Linux / macOS / Windows)
4. Run dependency check, print status for each dep with ✓/✗
5. Ask user: "Install missing dependencies? (y/n)"
6. If yes → extract and run the platform install script, show output
7. Write MCP server entry into `config.json` under `tools.mcp.servers`
8. Also set `tools.mcp.enabled = true` if not already
9. Print: "re-X enabled. Restart picoclawx to activate."

---

## Agent System (NEW)

### Predefined Workflows (`agent/workflows.go`)

```go
var Workflows = map[string][]string{
    "apk_triage":        {"extract_manifest", "decompile_apk", "search_strings", "list_apk_contents"},
    "protection_bypass": {"detect_protections", "patch_ssl_pinning", "patch_root_detection", "patch_emulator_detection", "rebuild_and_sign"},
    "dynamic_analysis":  {"frida_spawn", "frida_capture", "logcat"},
}
```

### Planner (`agent/planner.go`)

Converts free-form user intent into a structured execution plan.

```go
type Plan struct {
    Goal       string
    Steps      []Step
    Context    map[string]interface{}
    Confidence float64
}

type Step struct {
    Tool        string
    Params      map[string]interface{}
    RetryPolicy int
}
```

Selects workflows based on intent keywords, injects params (package name, apk path, mode).

### Execution Engine (`agent/executor.go`)

- Executes `Plan.Steps` sequentially
- Captures structured JSON output per step
- On failure: checks `recoverable`, applies `RetryPolicy`, calls evaluator for next-step decision
- Passes outputs forward as context for subsequent steps

### Evaluation Loop (`agent/evaluator.go`)

After each step result, evaluator:

1. Validates output structure
2. Detects failure type from `error_type`
3. Returns adapted next step or retry directive

Example adaptive strategy:

```
frida_attach fails (frida_detection) → retry with frida_spawn
frida_spawn fails                    → delay_hooks
still fails                          → fallback to static patching via patch_ssl_pinning
```

### Memory System (`agent/memory.go`) — Optional

```
~/.picoclaw/memory/
  apps/<package_name>.json    # previous findings per app
  patterns/                   # reusable detection patterns
```

Used to skip redundant analysis steps and reuse bypass patterns.

### LLM Interpreter (`agent/llm_interpreter.go`) ← NEW

After all tool steps complete, raw findings are passed to an LLM call that interprets them and generates the `insights` array. This replaces pure rule-based detection for the insight layer.

#### Why This Matters

Rule-based detection can identify *that* OkHttp is present, but cannot reason about *what that means* in context — e.g. whether the pinning is bypassable, what the attack surface looks like, or what order of operations to recommend. The LLM layer bridges raw data → actionable conclusions.

#### What It Receives (prompt input)

```json
{
  "package": "com.example.app",
  "manifest": { "permissions": [], "min_sdk": 21 },
  "protections": {
    "ssl_pinning": true,
    "ssl_pinning_method": "OkHttp",
    "root_detection": true,
    "root_detection_method": "Build.TAGS",
    "frida_detection": false,
    "obfuscation_level": "medium"
  },
  "strings_of_interest": ["api.example.com", "AES/ECB/PKCS5Padding"],
  "dynamic_logs_summary": "crypto hook triggered 3x, network hook captured 12 requests",
  "focus": "auth"
}
```

#### What It Returns

```json
{
  "insights": [
    "SSL pinning via OkHttp — smali patch is preferred; Frida hook also viable",
    "Root detection relies on Build.TAGS — a single smali edit removes this check",
    "AES/ECB detected — no IV, keys may be statically embedded, extractable via crypto-hooks",
    "12 network requests captured — recommend inspecting for auth token patterns"
  ],
  "recommended_next_steps": [
    "patch_ssl_pinning with strategy=smali",
    "patch_root_detection with strategy=smali",
    "run frida_capture with crypto-hooks for 60s"
  ],
  "risk_summary": "Medium obfuscation; static patching sufficient for most protections. No Frida detection — dynamic analysis safe to proceed."
}
```

#### Integration Points

- Called at the end of `run_analysis` after all tool steps complete
- Also callable standalone via `interpret_findings(workspace_path)` tool on `re-static.py`
- LLM provider configured via `~/.picoclaw/config.json` under `tools.llm`:

```json
"tools": {
  "llm": {
    "provider": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "api_key_env": "ANTHROPIC_API_KEY"
  }
}
```

- Supported providers: `anthropic`, `openai`, `ollama` (local/offline)
- If no LLM is configured, `insights` falls back to rule-based strings and a warning is added: `"insights_source": "rule-based"`

#### Config Written by `picoclawx llm configure` (new sub-command)

```
picoclawx llm configure   # interactive: choose provider, enter key, test connection
picoclawx llm test        # send a test prompt, print response
```

---

## Dependency Matrix

### re-static
- python3 (3.8+)
- pip
- mcp (pip)
- apktool (needs Java 8+)
- jadx (optional but preferred)

### re-device
- python3
- pip
- mcp (pip)
- adb (platform-tools) — on Termux: `pkg install android-tools`, works locally without USB

### re-dynamic
- python3
- pip
- mcp (pip)
- frida-tools (pip)
- adb (for non-Termux)
- frida-server on-device binary (arch-specific, auto-downloaded by install script)

---

## Platform Detection Logic (`detect.go`)

```
1. Check $PREFIX env var contains "com.termux" → Termux
2. Check /etc/os-release or uname → Linux
3. Check uname == Darwin → macOS
4. Check GOOS == windows → Windows
```

---

## Python MCP Servers

All three servers use the `mcp` pip package for stdio transport. All return structured JSON responses — never raw terminal output. Raw subprocess output goes to log files in the session workspace.

### Session Workspace Per Analysis

```
~/.picoclaw/re/<package_or_apk_name>_<timestamp>/
  original.apk
  apktool/
  jadx/
  dynamic/
    frida_ssl_unpin.log
    frida_method_trace.log
    frida_crypto.log
    logcat.log
  patched/                    ← NEW
    patched.apk
    patched-signed.apk
  report.json
  report.md
```

---

## re-static.py Tools

### `decompile_apk(apk_path, output_dir, tool)`
- `tool`: `"apktool"` | `"jadx"` | `"both"`
- Returns: `{status, output_dir, stats: {smali_files, native_libs, assets}, warnings}`

### `recompile_apk(source_dir, output_apk)`
- Returns: `{status, output_apk, size_bytes}`

### `sign_apk(apk_path, output_path)`
- Auto-generates debug keystore if none exists
- Returns: `{status, output_path, keystore_path}`

### `extract_manifest(apk_path)`
- Returns: `{status, package, version, permissions, activities, services, receivers, providers, min_sdk, target_sdk}`

### `list_apk_contents(apk_path)`
- Returns: `{status, files: [{path, size, type}], native_libs, interesting_files}`

### `read_smali(file_path)`
- Returns: `{status, content, line_count}`

### `search_strings(target, pattern, context_lines)`
- `target`: apk path or decompiled dir
- Returns: `{status, matches: [{file, line, content, context}], total_count}`

### `find_class(source_dir, class_name)`
- Returns: `{status, matches: [{file, class, methods}]}`

### `diff_smali(file_a, file_b)`
- Returns: `{status, diff, changed_lines}`

### `detect_protections(apk_path_or_source_dir)` ← NEW
- Scans smali and manifest for known protection patterns
- Returns:
```json
{
  "status": "success",
  "ssl_pinning": true,
  "root_detection": true,
  "emulator_detection": false,
  "frida_detection": true,
  "obfuscation_level": "medium",
  "details": {
    "ssl_pinning_method": "OkHttp",
    "root_detection_method": "Build.TAGS"
  }
}
```

### `patch_ssl_pinning(source_dir, strategy)` ← NEW
- `strategy`: `"smali"` | `"frida"` (smali modifies decompiled code directly)
- Returns: `{status, modified_files, patch_summary}`

### `patch_root_detection(source_dir, strategy)` ← NEW
- `strategy`: `"smali"` | `"frida"`
- Returns: `{status, modified_files, patch_summary}`

### `patch_emulator_detection(source_dir, strategy)` ← NEW
- Returns: `{status, modified_files, patch_summary}`

### `rebuild_and_sign(source_dir, output_path)` ← NEW
- Combines `recompile_apk` + `sign_apk` in one call
- Returns: `{status, output_path, size_bytes, keystore_path}`

### `interpret_findings(workspace_path, focus)` ← NEW
- Calls the configured LLM with all collected findings from the workspace
- `focus`: optional — `"crypto"` | `"network"` | `"auth"` | `"root-detection"` | `null`
- Falls back to rule-based insights if no LLM is configured
- Returns:
```json
{
  "status": "success",
  "insights": ["..."],
  "recommended_next_steps": ["..."],
  "risk_summary": "...",
  "insights_source": "llm" | "rule-based"
}
```

### `run_analysis(target, mode, focus)` ← META-TOOL (UPDATED)
- `target`: package name OR apk file path
- `mode`: `"full"` | `"static"` | `"dynamic"` | `"quick"`
- `focus`: `"crypto"` | `"network"` | `"auth"` | `"root-detection"` | `null`
- `"quick"`: manifest + string search + 10s dynamic capture (~2 min)
- `"full"`: everything (~10-15 min), invokes agent planner internally
- `"static"`: no device needed
- Now invokes the agent planner internally when `mode="full"` or `mode="dynamic"`
- Applies patches automatically if protections are detected
- **Calls `interpret_findings` (LLM interpreter) at the end of every run** to generate `insights`, `recommended_next_steps`, and `risk_summary` — falls back to rule-based if no LLM configured
- Runs full pipeline internally, streams progress, returns structured report
- Returns:
```json
{
  "status": "success",
  "workspace": "...",
  "patched_apk": "...",
  "findings": {
    "secrets": [],
    "endpoints": [],
    "permissions": [],
    "crypto": [],
    "obfuscation_level": "medium"
  },
  "insights": [
    "SSL pinning detected via OkHttp — bypass via smali patch or frida hook",
    "Root detection uses Build.TAGS — removable via smali edit",
    "Crypto uses insecure AES/ECB — keys likely extractable"
  ],
  "recommended_next_steps": [
    "patch_ssl_pinning with strategy=smali",
    "run frida_capture with crypto-hooks for 60s"
  ],
  "risk_summary": "Medium obfuscation; static patching sufficient for most protections.",
  "insights_source": "llm",
  "confidence": 0.87,
  "report_path": "..."
}
```

---

## re-device.py Tools

### `get_capabilities()`
- Returns: `{mode: "on-device"|"usb"|"static-only", device_id, android_version, is_rooted}`

### `list_devices()`
- Returns: `{devices: [{id, model, android_version, status}]}`

### `list_packages(filter)`
- Returns: `{packages: [{name, version, path, is_system}]}`

### `pull_apk(package_name, output_path)`
- Returns: `{status, output_path, size_bytes}`

### `install_apk(apk_path, replace)`
- Returns: `{status, package_name}`

### `uninstall_apk(package_name)`
- Returns: `{status}`

### `run_shell(command, timeout)`
- Uses `adb shell` on desktop, direct `su` on Termux
- Returns: `{status, stdout, stderr, exit_code}`

### `logcat(package_name, duration_seconds)`
- Returns: `{status, log_path, line_count, errors_found, warnings_found}`

### `get_app_data_path(package_name)`
- Returns: `{status, path, accessible}`

### `pull_file(device_path, local_path)`
- Returns: `{status, local_path, size_bytes}`

### `push_file(local_path, device_path)`
- Returns: `{status, device_path}`

---

## re-dynamic.py Tools

### `frida_list_processes()`
- Returns: `{processes: [{pid, name, package}]}`

### `frida_spawn(package_name, script, script_file, timeout)`
- `script`: built-in name OR null
- `script_file`: absolute path to user JS file OR null
- `script_file` takes priority over `script`
- Returns: `{status, log_path, findings_summary, raw_output_path}`

### `frida_attach(package_name, script, script_file, timeout)`
- Same params as `frida_spawn`
- Returns: same as `frida_spawn`

### `frida_capture(package_name, duration, output_file)`
- Runs `ssl-unpin` + `root-bypass` + `crypto-hooks` + `network-intercept` combined
- Returns: `{status, output_file, duration, hooks_triggered}`

### `list_builtin_scripts()`
- Returns: `{scripts: [{name, description, use_case}]}`

---

## Error Response Contract (ALL TOOLS) ← NEW

Every tool must return on failure:

```json
{
  "status": "error",
  "error_type": "frida_detection | apktool_failure | adb_not_found | ...",
  "recoverable": true,
  "suggested_next_steps": ["retry_with_spawn", "fallback_static_patch"]
}
```

This is mandatory — the executor relies on `recoverable` and `suggested_next_steps` to drive the adaptive strategy loop.

---

## Insight Layer ← NEW

All analysis results (from `run_analysis` and `interpret_findings`) must include an `insights` array. The **primary source is an LLM call** via `llm_interpreter.go` — the LLM reasons over the full collected findings to produce human-readable, actionable conclusions that rule-based pattern matching cannot. Rule-based strings are the fallback only when no LLM is configured.

```json
"insights": [
  "SSL pinning detected via OkHttp — bypass via smali patch or frida hook",
  "Root detection uses Build.TAGS — removable via smali edit",
  "Crypto uses insecure AES/ECB — keys likely extractable"
],
"insights_source": "llm"
```

---

## Built-in Frida Scripts

| Name | Description | Use Case |
|------|-------------|----------|
| `ssl-unpin` | Bypass SSL pinning (OkHttp, TrustManager, Conscrypt) | Traffic capture |
| `root-bypass` | Bypass common root detection | Run on rooted devices |
| `method-trace` | Trace all method calls on a class (parameterized) | Code flow analysis |
| `crypto-hooks` | Hook javax.crypto, log keys/plaintext | Key extraction |
| `network-intercept` | Log all HTTP/HTTPS via OkHttp/HttpURLConnection | Network analysis |
| `intent-monitor` | Log all intents fired | Component interaction |
| `shared-prefs` | Hook SharedPreferences read/write | Data storage analysis |

During `frida_capture` and `run_analysis(mode="dynamic")`, all scripts run simultaneously as a combined Frida script.

---

## Config Written by `enable` Command

```json
"tools": {
  "mcp": {
    "enabled": true,
    "servers": {
      "re-static": {
        "enabled": true,
        "command": "python3",
        "args": ["/home/user/.picoclaw/mcp/re/re-static.py"],
        "type": "stdio"
      },
      "re-device": {
        "enabled": true,
        "command": "python3",
        "args": ["/home/user/.picoclaw/mcp/re/re-device.py"],
        "type": "stdio"
      },
      "re-dynamic": {
        "enabled": true,
        "command": "python3",
        "args": ["/home/user/.picoclaw/mcp/re/re-dynamic.py"],
        "type": "stdio"
      }
    }
  }
}
```

Path uses actual expanded home dir, not `~`.

---

## Key Design Principles

- All tool responses are structured JSON — never raw terminal output
- Raw subprocess output goes to log files in the session workspace
- `run_analysis` is the primary entry point — drives the full agent pipeline autonomously
- Agent planner selects workflows from intent, executor runs steps, evaluator adapts on failure
- Platform differences hidden inside servers — agent calls same tools everywhere
- Patching is automatic when protections are detected during `run_analysis`
- **LLM interpreter runs at the end of every `run_analysis`** — raw findings in, actionable insights + next steps out; falls back to rule-based if no LLM configured
- Frida script priority: `script_file` (user path) first, then built-in `script` name
- Each analysis gets its own timestamped workspace under `~/.picoclaw/re/`
- Memory system (optional) enables faster re-analysis and pattern reuse
- Every tool must return a structured error with `recoverable` + `suggested_next_steps`

---

## Build Order

1. `cmd/picoclaw/internal/mcp/embed.go`
2. `cmd/picoclaw/internal/mcp/detect.go`
3. `cmd/picoclaw/internal/mcp/manager.go`
4. `cmd/picoclaw/internal/agent/workflows.go` ← NEW
5. `cmd/picoclaw/internal/agent/planner.go` ← NEW
6. `cmd/picoclaw/internal/agent/executor.go` ← NEW
7. `cmd/picoclaw/internal/agent/evaluator.go` ← NEW
8. `cmd/picoclaw/internal/agent/llm_interpreter.go` ← NEW
9. `cmd/picoclaw/internal/agent/memory.go` ← NEW (optional)
10. `cmd/picoclaw/mcp_cmd.go`
11. `cmd/picoclaw/llm_cmd.go` ← NEW (`picoclawx llm configure` / `picoclawx llm test`)
12. `mcp/re/install/termux.sh`, `linux.sh`, `macos.sh`, `windows.bat`
13. `mcp/re/scripts/*.js` — all seven Frida scripts
14. `mcp/re/re-static.py` (includes detect/patch/rebuild/interpret_findings tools + updated run_analysis)
15. `mcp/re/re-device.py`
16. `mcp/re/re-dynamic.py`