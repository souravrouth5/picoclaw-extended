#!/usr/bin/env python3
"""re-device: Device interaction MCP server for PicoClaw Extended."""

import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

from mcp.server.fastmcp import FastMCP

app = FastMCP("re-device")

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _home() -> Path:
    return Path.home() / ".picoclaw"

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

def _adb(args: list, timeout=30) -> subprocess.CompletedProcess:
    adb = shutil.which("adb") or "adb"
    return _run([adb] + args, timeout=timeout)

def _adb_ok() -> bool:
    try:
        r = _adb(["version"])
        return r.returncode == 0
    except Exception:
        return False

def _parse_devices(output: str) -> list:
    devices = []
    for line in output.splitlines()[1:]:
        line = line.strip()
        if not line or "List of devices" in line:
            continue
        parts = line.split()
        if len(parts) >= 2:
            devices.append({"id": parts[0], "status": parts[1]})
    return devices

# ---------------------------------------------------------------------------
# Tool implementations
# ---------------------------------------------------------------------------

def _get_capabilities() -> dict:
    if _is_termux():
        # On-device: check if we have root
        r = _run(["su", "-c", "id"], timeout=5)
        is_rooted = r.returncode == 0
        # Get android version from prop
        r2 = _run(["getprop", "ro.build.version.release"])
        android_ver = r2.stdout.strip() if r2.returncode == 0 else "unknown"
        return _ok(
            mode="on-device",
            device_id="localhost",
            android_version=android_ver,
            is_rooted=is_rooted,
        )

    if not _adb_ok():
        return _ok(mode="static-only", device_id=None, android_version=None, is_rooted=False)

    r = _adb(["devices"])
    devices = _parse_devices(r.stdout)
    connected = [d for d in devices if d["status"] == "device"]
    if not connected:
        return _ok(mode="static-only", device_id=None, android_version=None, is_rooted=False)

    dev_id = connected[0]["id"]
    r2 = _adb(["-s", dev_id, "shell", "getprop", "ro.build.version.release"])
    android_ver = r2.stdout.strip() if r2.returncode == 0 else "unknown"
    r3 = _adb(["-s", dev_id, "shell", "su", "-c", "id"])
    is_rooted = r3.returncode == 0

    return _ok(mode="usb", device_id=dev_id, android_version=android_ver, is_rooted=is_rooted)


def _list_devices() -> dict:
    if _is_termux():
        r = _run(["getprop", "ro.build.version.release"])
        ver = r.stdout.strip() if r.returncode == 0 else "unknown"
        r2 = _run(["getprop", "ro.product.model"])
        model = r2.stdout.strip() if r2.returncode == 0 else "unknown"
        return _ok(devices=[{"id": "localhost", "model": model, "android_version": ver, "status": "device"}])

    if not _adb_ok():
        return _err("adb_not_found", "adb not found in PATH", True, ["install platform-tools"])

    r = _adb(["devices", "-l"])
    if r.returncode != 0:
        return _err("adb_error", r.stderr[:200], True)

    devices = []
    for line in r.stdout.splitlines()[1:]:
        line = line.strip()
        if not line:
            continue
        parts = line.split()
        if len(parts) < 2:
            continue
        dev_id = parts[0]
        status = parts[1]
        model = ""
        for p in parts[2:]:
            if p.startswith("model:"):
                model = p[6:]
        ver = ""
        if status == "device":
            r2 = _adb(["-s", dev_id, "shell", "getprop", "ro.build.version.release"])
            ver = r2.stdout.strip() if r2.returncode == 0 else ""
        devices.append({"id": dev_id, "model": model, "android_version": ver, "status": status})

    return _ok(devices=devices)


def _list_packages(filter_str: str = "") -> dict:
    if _is_termux():
        r = _run(["pm", "list", "packages", "-f"])
    else:
        if not _adb_ok():
            return _err("adb_not_found", "adb not found", True, ["install platform-tools"])
        caps = _get_capabilities()
        dev_id = caps.get("device_id")
        if not dev_id:
            return _err("device_not_connected", "No device connected", True)
        r = _adb(["-s", dev_id, "shell", "pm", "list", "packages", "-f"])

    if r.returncode != 0:
        return _err("pm_error", r.stderr[:200], True)

    packages = []
    for line in r.stdout.splitlines():
        m = re.match(r"package:(.+)=(.+)", line.strip())
        if not m:
            continue
        path, name = m.group(1), m.group(2)
        if filter_str and filter_str.lower() not in name.lower():
            continue
        packages.append({"name": name, "path": path, "is_system": path.startswith("/system")})

    return _ok(packages=packages)


def _pull_apk(package_name: str, output_path: str = None) -> dict:
    # Find APK path
    pkgs = _list_packages(package_name)
    if pkgs["status"] != "success":
        return pkgs
    match = [p for p in pkgs["packages"] if p["name"] == package_name]
    if not match:
        return _err("package_not_found", f"Package not found: {package_name}", False)

    apk_device_path = match[0]["path"]
    if not output_path:
        output_path = str(_home() / "re" / f"{package_name}.apk")
    Path(output_path).parent.mkdir(parents=True, exist_ok=True)

    if _is_termux():
        r = _run(["cp", apk_device_path, output_path])
    else:
        caps = _get_capabilities()
        dev_id = caps.get("device_id")
        if not dev_id:
            return _err("device_not_connected", "No device connected", True)
        r = _adb(["-s", dev_id, "pull", apk_device_path, output_path])

    if r.returncode != 0:
        return _err("pull_failed", r.stderr[:200], True)

    size = Path(output_path).stat().st_size if Path(output_path).exists() else 0
    return _ok(output_path=output_path, size_bytes=size)


def _install_apk(apk_path: str, replace: bool = True) -> dict:
    apk = Path(apk_path)
    if not apk.exists():
        return _err("file_not_found", f"APK not found: {apk_path}", False)

    if _is_termux():
        cmd = ["pm", "install"]
        if replace:
            cmd.append("-r")
        cmd.append(str(apk))
        r = _run(cmd)
    else:
        if not _adb_ok():
            return _err("adb_not_found", "adb not found", True)
        caps = _get_capabilities()
        dev_id = caps.get("device_id")
        if not dev_id:
            return _err("device_not_connected", "No device connected", True)
        cmd = ["-s", dev_id, "install"]
        if replace:
            cmd.append("-r")
        cmd.append(str(apk))
        r = _adb(cmd, timeout=60)

    if r.returncode != 0:
        return _err("install_failed", r.stdout[:200] + r.stderr[:200], True)

    # Extract package name from output
    pkg = re.search(r"pkg: (.+)", r.stdout)
    return _ok(package_name=pkg.group(1).strip() if pkg else "")


def _uninstall_apk(package_name: str) -> dict:
    if _is_termux():
        r = _run(["pm", "uninstall", package_name])
    else:
        if not _adb_ok():
            return _err("adb_not_found", "adb not found", True)
        caps = _get_capabilities()
        dev_id = caps.get("device_id")
        if not dev_id:
            return _err("device_not_connected", "No device connected", True)
        r = _adb(["-s", dev_id, "uninstall", package_name])

    if r.returncode != 0:
        return _err("uninstall_failed", r.stderr[:200], True)
    return _ok()

def _run_shell(command: str, timeout: int = 30) -> dict:
    if _is_termux():
        r = _run(["sh", "-c", command], timeout=timeout)
    else:
        if not _adb_ok():
            return _err("adb_not_found", "adb not found", True)
        caps = _get_capabilities()
        dev_id = caps.get("device_id")
        if not dev_id:
            return _err("device_not_connected", "No device connected", True)
        r = _adb(["-s", dev_id, "shell", command], timeout=timeout)

    return _ok(stdout=r.stdout[:4000], stderr=r.stderr[:1000], exit_code=r.returncode)


def _logcat(package_name: str = None, duration_seconds: int = 10) -> dict:
    ws = Path.home() / ".picoclaw" / "re"
    ws.mkdir(parents=True, exist_ok=True)
    log_path = str(ws / f"logcat_{package_name or 'all'}.log")

    if _is_termux():
        cmd = ["logcat", "-d"]
        if package_name:
            cmd += ["--pid", f"$(pidof {package_name})"]
    else:
        if not _adb_ok():
            return _err("adb_not_found", "adb not found", True)
        caps = _get_capabilities()
        dev_id = caps.get("device_id")
        if not dev_id:
            return _err("device_not_connected", "No device connected", True)
        cmd = ["adb", "-s", dev_id, "logcat", "-d"]
        if package_name:
            cmd += [f"--pid=$(adb -s {dev_id} shell pidof {package_name})"]

    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=duration_seconds + 5)
        lines = r.stdout.splitlines()
        Path(log_path).write_text(r.stdout)
        errors = sum(1 for l in lines if " E " in l)
        warnings = sum(1 for l in lines if " W " in l)
        return _ok(log_path=log_path, line_count=len(lines), errors_found=errors, warnings_found=warnings)
    except subprocess.TimeoutExpired:
        return _err("timeout", "logcat timed out", True)
    except Exception as e:
        return _err("logcat_error", str(e), True)


def _get_app_data_path(package_name: str) -> dict:
    path = f"/data/data/{package_name}"
    if _is_termux():
        accessible = Path(path).exists()
    else:
        if not _adb_ok():
            return _err("adb_not_found", "adb not found", True)
        caps = _get_capabilities()
        dev_id = caps.get("device_id")
        if not dev_id:
            return _err("device_not_connected", "No device connected", True)
        r = _adb(["-s", dev_id, "shell", f"ls {path}"])
        accessible = r.returncode == 0
    return _ok(path=path, accessible=accessible)


def _pull_file(device_path: str, local_path: str) -> dict:
    Path(local_path).parent.mkdir(parents=True, exist_ok=True)
    if _is_termux():
        r = _run(["cp", device_path, local_path])
    else:
        if not _adb_ok():
            return _err("adb_not_found", "adb not found", True)
        caps = _get_capabilities()
        dev_id = caps.get("device_id")
        if not dev_id:
            return _err("device_not_connected", "No device connected", True)
        r = _adb(["-s", dev_id, "pull", device_path, local_path])

    if r.returncode != 0:
        return _err("pull_failed", r.stderr[:200], True)
    size = Path(local_path).stat().st_size if Path(local_path).exists() else 0
    return _ok(local_path=local_path, size_bytes=size)


def _push_file(local_path: str, device_path: str) -> dict:
    if not Path(local_path).exists():
        return _err("file_not_found", f"File not found: {local_path}", False)
    if _is_termux():
        r = _run(["cp", local_path, device_path])
    else:
        if not _adb_ok():
            return _err("adb_not_found", "adb not found", True)
        caps = _get_capabilities()
        dev_id = caps.get("device_id")
        if not dev_id:
            return _err("device_not_connected", "No device connected", True)
        r = _adb(["-s", dev_id, "push", local_path, device_path])

    if r.returncode != 0:
        return _err("push_failed", r.stderr[:200], True)
    return _ok(device_path=device_path)


# ---------------------------------------------------------------------------
# MCP tool registry
# ---------------------------------------------------------------------------

@app.tool()
def get_capabilities() -> str:
    return json.dumps(_get_capabilities(), indent=2)

@app.tool()
def list_devices() -> str:
    return json.dumps(_list_devices(), indent=2)

@app.tool()
def list_packages(filter: str = "") -> str:
    return json.dumps(_list_packages(filter), indent=2)

@app.tool()
def pull_apk(package_name: str, output_path: str = "") -> str:
    return json.dumps(_pull_apk(package_name, output_path or None), indent=2)

@app.tool()
def install_apk(apk_path: str, replace: bool = True) -> str:
    return json.dumps(_install_apk(apk_path, replace), indent=2)

@app.tool()
def uninstall_apk(package_name: str) -> str:
    return json.dumps(_uninstall_apk(package_name), indent=2)

@app.tool()
def run_shell(command: str, timeout: int = 30) -> str:
    return json.dumps(_run_shell(command, timeout), indent=2)

@app.tool()
def logcat(package_name: str = "", duration_seconds: int = 10) -> str:
    return json.dumps(_logcat(package_name or None, duration_seconds), indent=2)

@app.tool()
def get_app_data_path(package_name: str) -> str:
    return json.dumps(_get_app_data_path(package_name), indent=2)

@app.tool()
def pull_file(device_path: str, local_path: str) -> str:
    return json.dumps(_pull_file(device_path, local_path), indent=2)

@app.tool()
def push_file(local_path: str, device_path: str) -> str:
    return json.dumps(_push_file(local_path, device_path), indent=2)


if __name__ == "__main__":
    app.run()
