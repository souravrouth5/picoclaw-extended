#!/usr/bin/env python3
"""re-dynamic: Dynamic analysis MCP server for PicoClaw Extended."""

import json
import os
import re
import shutil
import subprocess
import sys
import threading
import time
from datetime import datetime
from pathlib import Path

from mcp.server.fastmcp import FastMCP

app = FastMCP("re-dynamic")

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _home() -> Path:
    return Path.home() / ".picoclaw"

def _scripts_dir() -> Path:
    return _home() / "mcp" / "re" / "scripts"

def _ws() -> Path:
    ts = datetime.now().strftime("%Y%m%d_%H%M%S")
    p = _home() / "re" / f"dynamic_{ts}"
    p.mkdir(parents=True, exist_ok=True)
    return p

def _ok(**kwargs) -> dict:
    return {"status": "success", **kwargs}

def _err(error_type: str, msg: str, recoverable: bool = True, next_steps: list = None) -> dict:
    return {
        "status": "error",
        "error_type": error_type,
        "message": msg,
        "recoverable": recoverable,
        "suggested_next_steps": next_steps or [],
    }

def _is_termux() -> bool:
    return "com.termux" in os.environ.get("PREFIX", "")

def _run(cmd: list, timeout=30) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)

def _frida_cmd() -> str:
    return shutil.which("frida") or "frida"

def _frida_ps_cmd() -> str:
    return shutil.which("frida-ps") or "frida-ps"

# Built-in script metadata
_BUILTIN_SCRIPTS = {
    "ssl-unpin":         "Bypass SSL pinning (OkHttp, TrustManager, Conscrypt)",
    "root-bypass":       "Bypass common root detection checks",
    "method-trace":      "Trace all method calls on a class (parameterized)",
    "crypto-hooks":      "Hook javax.crypto — log keys, IV, plaintext",
    "network-intercept": "Log all HTTP/HTTPS via OkHttp/HttpURLConnection",
    "intent-monitor":    "Log all intents fired by the app",
    "shared-prefs":      "Hook SharedPreferences read/write operations",
}

def _load_script(script_file: str = None, script: str = None) -> tuple[str, str]:
    """Returns (js_content, script_name). script_file takes priority."""
    if script_file:
        p = Path(script_file)
        if not p.exists():
            raise FileNotFoundError(f"Script file not found: {script_file}")
        return p.read_text(), p.name

    if script:
        p = _scripts_dir() / f"{script}.js"
        if not p.exists():
            raise FileNotFoundError(f"Built-in script not found: {script}. Run: picoclawx mcp enable re-dynamic")
        return p.read_text(), script

    raise ValueError("Either script or script_file must be provided")


def _combine_scripts(names: list) -> str:
    """Combine multiple built-in scripts into one JS string."""
    parts = []
    for name in names:
        p = _scripts_dir() / f"{name}.js"
        if p.exists():
            parts.append(f"// === {name} ===\n" + p.read_text())
    return "\n\n".join(parts)

# ---------------------------------------------------------------------------
# Tool implementations
# ---------------------------------------------------------------------------

def _frida_list_processes() -> dict:
    frida_ps = _frida_ps_cmd()
    try:
        if _is_termux():
            r = _run([frida_ps], timeout=10)
        else:
            r = _run([frida_ps, "-U"], timeout=10)
    except FileNotFoundError:
        return _err("frida_not_found", "frida-ps not found", True, ["picoclawx mcp install-deps re-dynamic"])
    except subprocess.TimeoutExpired:
        return _err("timeout", "frida-ps timed out", True)

    if r.returncode != 0:
        return _err("frida_error", r.stderr[:300], True, ["ensure frida-server is running on device"])

    processes = []
    for line in r.stdout.splitlines()[1:]:
        parts = line.split(None, 2)
        if len(parts) >= 2:
            processes.append({
                "pid": parts[0],
                "name": parts[1],
                "package": parts[2].strip() if len(parts) > 2 else parts[1],
            })
    return _ok(processes=processes)


def _run_frida(mode: str, package_name: str, js: str, script_name: str,
               timeout: int, log_path: str) -> dict:
    """Core Frida runner used by both spawn and attach."""
    frida = _frida_cmd()
    if not shutil.which(frida):
        return _err("frida_not_found", "frida not found", True,
                    ["picoclawx mcp install-deps re-dynamic"])

    # Write script to temp file
    tmp_script = Path(log_path).parent / f"_script_{script_name}.js"
    tmp_script.write_text(js)

    cmd = [frida]
    if not _is_termux():
        cmd += ["-U"]
    if mode == "spawn":
        cmd += ["-f", package_name, "--no-pause"]
    else:
        cmd += [package_name]
    cmd += ["-l", str(tmp_script), "--runtime=v8"]

    raw_log = Path(log_path).parent / f"raw_{script_name}.log"

    try:
        proc = subprocess.Popen(
            cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
        )
        try:
            stdout, stderr = proc.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            proc.kill()
            stdout, stderr = proc.communicate()

        raw_log.write_text(stdout + "\n--- STDERR ---\n" + stderr)

        # Parse findings summary from output
        hooks_triggered = len(re.findall(r"\[hook\]", stdout, re.I))
        findings = {
            "hooks_triggered": hooks_triggered,
            "lines_captured": len(stdout.splitlines()),
        }

        # Write structured log
        Path(log_path).write_text(json.dumps({
            "script": script_name,
            "package": package_name,
            "mode": mode,
            "findings": findings,
            "output_preview": stdout[:2000],
        }, indent=2))

        if proc.returncode not in (0, -15, None) and "frida" in stderr.lower():
            if "unable to find process" in stderr.lower():
                return _err("process_not_found", stderr[:200], True,
                            ["start the app first", "use frida_spawn instead"])
            if "detection" in stderr.lower() or "gadget" in stderr.lower():
                return _err("frida_detection", stderr[:200], True,
                            ["retry_with_spawn", "use delay_hooks"])

        return _ok(
            log_path=log_path,
            findings_summary=findings,
            raw_output_path=str(raw_log),
        )
    except FileNotFoundError:
        return _err("frida_not_found", "frida binary not found", True,
                    ["picoclawx mcp install-deps re-dynamic"])
    except Exception as e:
        return _err("frida_error", str(e), True)
    finally:
        tmp_script.unlink(missing_ok=True)


def _frida_spawn(package_name: str, script: str = None, script_file: str = None,
                 timeout: int = 30) -> dict:
    try:
        js, name = _load_script(script_file, script)
    except Exception as e:
        return _err("script_error", str(e), False)

    ws = _ws()
    log_path = str(ws / f"frida_spawn_{name}.log")
    return _run_frida("spawn", package_name, js, name, timeout, log_path)


def _frida_attach(package_name: str, script: str = None, script_file: str = None,
                  timeout: int = 30) -> dict:
    try:
        js, name = _load_script(script_file, script)
    except Exception as e:
        return _err("script_error", str(e), False)

    ws = _ws()
    log_path = str(ws / f"frida_attach_{name}.log")
    return _run_frida("attach", package_name, js, name, timeout, log_path)

def _frida_capture(package_name: str, duration: int = 30, output_file: str = None) -> dict:
    """Run ssl-unpin + root-bypass + crypto-hooks + network-intercept combined."""
    capture_scripts = ["ssl-unpin", "root-bypass", "crypto-hooks", "network-intercept"]
    try:
        js = _combine_scripts(capture_scripts)
    except Exception as e:
        return _err("script_error", str(e), False)

    if not js.strip():
        return _err("scripts_not_extracted",
                    "Frida scripts not found. Run: picoclawx mcp enable re-dynamic",
                    True, ["picoclawx mcp enable re-dynamic"])

    ws = _ws()
    log_path = output_file or str(ws / "frida_capture.log")
    result = _run_frida("spawn", package_name, js, "capture", duration, log_path)

    if result["status"] == "success":
        findings = result.get("findings_summary", {})
        result["hooks_triggered"] = findings.get("hooks_triggered", 0)
        result["duration"] = duration
        result["output_file"] = log_path

    return result


def _list_builtin_scripts() -> dict:
    scripts = []
    for name, desc in _BUILTIN_SCRIPTS.items():
        use_cases = {
            "ssl-unpin":         "Traffic capture, MITM proxy setup",
            "root-bypass":       "Run app on rooted device without detection",
            "method-trace":      "Code flow analysis, understanding app logic",
            "crypto-hooks":      "Key extraction, plaintext recovery",
            "network-intercept": "Network analysis without proxy",
            "intent-monitor":    "Component interaction analysis",
            "shared-prefs":      "Data storage analysis",
        }
        extracted = (_scripts_dir() / f"{name}.js").exists()
        scripts.append({
            "name": name,
            "description": desc,
            "use_case": use_cases.get(name, ""),
            "available": extracted,
        })
    return _ok(scripts=scripts)


# ---------------------------------------------------------------------------
# MCP tool registry
# ---------------------------------------------------------------------------

@app.tool()
def frida_list_processes() -> str:
    return json.dumps(_frida_list_processes(), indent=2)

@app.tool()
def frida_spawn(package_name: str, script: str = "", script_file: str = "", timeout: int = 30) -> str:
    return json.dumps(_frida_spawn(package_name, script or None, script_file or None, timeout), indent=2)

@app.tool()
def frida_attach(package_name: str, script: str = "", script_file: str = "", timeout: int = 30) -> str:
    return json.dumps(_frida_attach(package_name, script or None, script_file or None, timeout), indent=2)

@app.tool()
def frida_capture(package_name: str, duration: int = 30, output_file: str = "") -> str:
    return json.dumps(_frida_capture(package_name, duration, output_file or None), indent=2)

@app.tool()
def list_builtin_scripts() -> str:
    return json.dumps(_list_builtin_scripts(), indent=2)


if __name__ == "__main__":
    app.run()
