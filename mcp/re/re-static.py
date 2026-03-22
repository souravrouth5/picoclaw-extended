#!/usr/bin/env python3
"""re-static: Static analysis MCP server for PicoClaw Extended."""

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile
from datetime import datetime
from pathlib import Path

from mcp.server.fastmcp import FastMCP

app = FastMCP("re-static")

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _home() -> Path:
    return Path.home() / ".picoclaw"

def _workspace(name: str) -> Path:
    ts = datetime.now().strftime("%Y%m%d_%H%M%S")
    ws = _home() / "re" / f"{name}_{ts}"
    ws.mkdir(parents=True, exist_ok=True)
    return ws

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

def _run(cmd: list, cwd=None, timeout=120) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, capture_output=True, text=True, cwd=cwd, timeout=timeout)

def _apktool_cmd() -> list:
    if shutil.which("apktool"):
        return ["apktool"]
    jar = Path.home() / ".picoclaw" / "tools" / "apktool.jar"
    if jar.exists():
        return ["java", "-jar", str(jar)]
    return ["apktool"]

def _jadx_cmd() -> list:
    if shutil.which("jadx"):
        return ["jadx"]
    jar = Path.home() / ".picoclaw" / "tools" / "jadx" / "bin" / "jadx"
    if Path(jar).exists():
        return [str(jar)]
    return ["jadx"]

def _keytool_cmd() -> str:
    return shutil.which("keytool") or "keytool"

def _apksigner_or_jarsigner() -> list:
    if shutil.which("apksigner"):
        return ["apksigner", "sign"]
    return None  # fall back to jarsigner

def _count_files(directory: Path, suffix: str) -> int:
    return sum(1 for _ in directory.rglob(f"*{suffix}"))

# ---------------------------------------------------------------------------
# Tool implementations
# ---------------------------------------------------------------------------

def _extract_manifest(apk_path: str) -> dict:
    apk = Path(apk_path)
    if not apk.exists():
        return _err("file_not_found", f"APK not found: {apk_path}", False)
    try:
        cmd = _apktool_cmd() + ["d", str(apk), "-o", str(apk.parent / "_manifest_tmp"), "-f", "--no-src"]
        r = _run(cmd)
        manifest_path = apk.parent / "_manifest_tmp" / "AndroidManifest.xml"
        if not manifest_path.exists():
            shutil.rmtree(apk.parent / "_manifest_tmp", ignore_errors=True)
            return _err("apktool_failure", r.stderr[:500], True, ["decompile_apk"])
        content = manifest_path.read_text(errors="replace")
        shutil.rmtree(apk.parent / "_manifest_tmp", ignore_errors=True)

        def _findall(tag):
            return re.findall(rf'<{tag}[^>]*android:name="([^"]+)"', content)

        pkg = re.search(r'package="([^"]+)"', content)
        ver = re.search(r'android:versionName="([^"]+)"', content)
        min_sdk = re.search(r'android:minSdkVersion="([^"]+)"', content)
        tgt_sdk = re.search(r'android:targetSdkVersion="([^"]+)"', content)
        perms = re.findall(r'uses-permission[^>]*android:name="([^"]+)"', content)

        return _ok(
            package=pkg.group(1) if pkg else "",
            version=ver.group(1) if ver else "",
            permissions=perms,
            activities=_findall("activity"),
            services=_findall("service"),
            receivers=_findall("receiver"),
            providers=_findall("provider"),
            min_sdk=min_sdk.group(1) if min_sdk else "",
            target_sdk=tgt_sdk.group(1) if tgt_sdk else "",
        )
    except Exception as e:
        return _err("apktool_failure", str(e), True, ["check_java", "install-deps re-static"])


def _list_apk_contents(apk_path: str) -> dict:
    apk = Path(apk_path)
    if not apk.exists():
        return _err("file_not_found", f"APK not found: {apk_path}", False)
    try:
        files = []
        native_libs = []
        interesting = []
        interesting_patterns = re.compile(
            r"\.(db|sqlite|so|json|xml|key|pem|cer|crt|p12|pfx|properties|conf|cfg)$", re.I
        )
        with zipfile.ZipFile(str(apk)) as z:
            for info in z.infolist():
                ftype = "other"
                name = info.filename
                if name.endswith(".dex"):
                    ftype = "dex"
                elif name.endswith(".so"):
                    ftype = "native"
                    native_libs.append(name)
                elif name.startswith("res/"):
                    ftype = "resource"
                elif name.startswith("assets/"):
                    ftype = "asset"
                files.append({"path": name, "size": info.file_size, "type": ftype})
                if interesting_patterns.search(name):
                    interesting.append(name)
        return _ok(files=files, native_libs=native_libs, interesting_files=interesting)
    except Exception as e:
        return _err("zip_error", str(e), False)


def _decompile_apk(apk_path: str, output_dir: str = None, tool: str = "both") -> dict:
    apk = Path(apk_path)
    if not apk.exists():
        return _err("file_not_found", f"APK not found: {apk_path}", False)

    if not output_dir:
        ws = _workspace(apk.stem)
        output_dir = str(ws)

    out = Path(output_dir)
    warnings = []
    stats = {}

    if tool in ("apktool", "both"):
        apktool_out = out / "apktool"
        r = _run(_apktool_cmd() + ["d", str(apk), "-o", str(apktool_out), "-f"])
        if r.returncode != 0:
            if tool == "apktool":
                return _err("apktool_failure", r.stderr[:500], True, ["decompile_apk with tool=jadx"])
            warnings.append(f"apktool failed: {r.stderr[:200]}")
        else:
            stats["smali_files"] = _count_files(apktool_out, ".smali")
            stats["native_libs"] = _count_files(apktool_out, ".so")
            stats["assets"] = _count_files(apktool_out / "assets", "")

    if tool in ("jadx", "both"):
        jadx_out = out / "jadx"
        r = _run(_jadx_cmd() + [str(apk), "-d", str(jadx_out)])
        if r.returncode != 0:
            if tool == "jadx":
                return _err("jadx_failure", r.stderr[:500], True, ["decompile_apk with tool=apktool"])
            warnings.append(f"jadx failed: {r.stderr[:200]}")
        else:
            stats["java_files"] = _count_files(jadx_out, ".java")

    return _ok(output_dir=str(out), stats=stats, warnings=warnings)

def _read_smali(file_path: str) -> dict:
    p = Path(file_path)
    if not p.exists():
        return _err("file_not_found", f"File not found: {file_path}", False)
    try:
        content = p.read_text(errors="replace")
        return _ok(content=content, line_count=content.count("\n") + 1)
    except Exception as e:
        return _err("read_error", str(e), False)


def _search_strings(target: str, pattern: str, context_lines: int = 2) -> dict:
    t = Path(target)
    if not t.exists():
        return _err("file_not_found", f"Target not found: {target}", False)

    try:
        rx = re.compile(pattern, re.IGNORECASE)
    except re.error as e:
        return _err("invalid_pattern", str(e), False)

    matches = []
    files = [t] if t.is_file() else list(t.rglob("*"))
    for f in files:
        if not f.is_file():
            continue
        try:
            lines = f.read_text(errors="replace").splitlines()
        except Exception:
            continue
        for i, line in enumerate(lines):
            if rx.search(line):
                start = max(0, i - context_lines)
                end = min(len(lines), i + context_lines + 1)
                matches.append({
                    "file": str(f),
                    "line": i + 1,
                    "content": line.strip(),
                    "context": lines[start:end],
                })
    return _ok(matches=matches, total_count=len(matches))


def _find_class(source_dir: str, class_name: str) -> dict:
    d = Path(source_dir)
    if not d.exists():
        return _err("file_not_found", f"Directory not found: {source_dir}", False)

    results = []
    pattern = class_name.replace(".", "/").lower()
    for f in d.rglob("*.smali"):
        if pattern in str(f).lower():
            try:
                content = f.read_text(errors="replace")
                methods = re.findall(r"\.method[^\n]+\n", content)
                results.append({
                    "file": str(f),
                    "class": f.stem,
                    "methods": [m.strip() for m in methods],
                })
            except Exception:
                pass
    for f in d.rglob("*.java"):
        if pattern in str(f).lower():
            try:
                content = f.read_text(errors="replace")
                methods = re.findall(r"(?:public|private|protected)[^\n{]+\(", content)
                results.append({
                    "file": str(f),
                    "class": f.stem,
                    "methods": [m.strip() for m in methods],
                })
            except Exception:
                pass
    return _ok(matches=results)


def _diff_smali(file_a: str, file_b: str) -> dict:
    a, b = Path(file_a), Path(file_b)
    for p in (a, b):
        if not p.exists():
            return _err("file_not_found", f"File not found: {p}", False)
    lines_a = a.read_text(errors="replace").splitlines()
    lines_b = b.read_text(errors="replace").splitlines()
    diff = []
    changed = 0
    max_len = max(len(lines_a), len(lines_b))
    for i in range(max_len):
        la = lines_a[i] if i < len(lines_a) else ""
        lb = lines_b[i] if i < len(lines_b) else ""
        if la != lb:
            diff.append({"line": i + 1, "a": la, "b": lb})
            changed += 1
    return _ok(diff=diff, changed_lines=changed)

# ---------------------------------------------------------------------------
# Protection detection & patching
# ---------------------------------------------------------------------------

_SSL_PATTERNS = [
    "CertificatePinner", "TrustManager", "checkServerTrusted",
    "ssl_pinning", "X509TrustManager", "HostnameVerifier",
    "OkHttpClient", "certificatePinner",
]
_ROOT_PATTERNS = [
    "Build.TAGS", "test-keys", "su", "RootBeer", "isRooted",
    "superuser", "busybox", "Magisk", "SuperSU",
]
_EMU_PATTERNS = [
    "Build.FINGERPRINT", "generic", "emulator", "genymotion",
    "Build.MODEL", "sdk_gphone", "Build.HARDWARE", "goldfish",
]
_FRIDA_PATTERNS = [
    "frida", "gadget", "FRIDA", "re.frida", "gum-js-loop",
]
_OBFUSCATION_CLASSES = re.compile(r"^[a-z]{1,3}$")


def _detect_protections(apk_path_or_dir: str) -> dict:
    target = Path(apk_path_or_dir)
    if not target.exists():
        return _err("file_not_found", f"Not found: {apk_path_or_dir}", False)

    # If it's an APK, decompile to temp dir first
    tmp_dir = None
    scan_dir = target
    if target.is_file() and target.suffix.lower() == ".apk":
        tmp_dir = Path(tempfile.mkdtemp())
        r = _run(_apktool_cmd() + ["d", str(target), "-o", str(tmp_dir), "-f"])
        if r.returncode != 0:
            shutil.rmtree(tmp_dir, ignore_errors=True)
            return _err("apktool_failure", r.stderr[:300], True, ["decompile_apk"])
        scan_dir = tmp_dir

    try:
        all_text = []
        class_names = []
        for f in scan_dir.rglob("*.smali"):
            try:
                t = f.read_text(errors="replace")
                all_text.append(t)
                m = re.search(r"\.class[^\n]+L([^;]+);", t)
                if m:
                    class_names.append(m.group(1).split("/")[-1])
            except Exception:
                pass

        combined = "\n".join(all_text)

        def _has(patterns):
            return any(p in combined for p in patterns)

        def _method(patterns):
            for p in patterns:
                if p in combined:
                    return p
            return ""

        ssl = _has(_SSL_PATTERNS)
        root = _has(_ROOT_PATTERNS)
        emu = _has(_EMU_PATTERNS)
        frida = _has(_FRIDA_PATTERNS)

        short_names = sum(1 for c in class_names if _OBFUSCATION_CLASSES.match(c))
        obf_ratio = short_names / max(len(class_names), 1)
        if obf_ratio > 0.5:
            obf_level = "high"
        elif obf_ratio > 0.2:
            obf_level = "medium"
        else:
            obf_level = "low"

        return _ok(
            ssl_pinning=ssl,
            root_detection=root,
            emulator_detection=emu,
            frida_detection=frida,
            obfuscation_level=obf_level,
            details={
                "ssl_pinning_method": _method(_SSL_PATTERNS),
                "root_detection_method": _method(_ROOT_PATTERNS),
            },
        )
    finally:
        if tmp_dir:
            shutil.rmtree(tmp_dir, ignore_errors=True)


def _patch_ssl_pinning(source_dir: str, strategy: str = "smali") -> dict:
    d = Path(source_dir)
    if not d.exists():
        return _err("file_not_found", f"Source dir not found: {source_dir}", False)

    modified = []
    if strategy == "smali":
        # Patch TrustManager checkServerTrusted to return immediately
        for f in d.rglob("*.smali"):
            try:
                content = f.read_text(errors="replace")
                if "checkServerTrusted" not in content:
                    continue
                # Replace method body with empty return
                patched = re.sub(
                    r"(\.method[^\n]*checkServerTrusted[^\n]*\n)(.+?)(\.end method)",
                    r"\1    return-void\n\3",
                    content,
                    flags=re.DOTALL,
                )
                if patched != content:
                    f.write_text(patched)
                    modified.append(str(f))
            except Exception:
                pass
        # Patch CertificatePinner check
        for f in d.rglob("*.smali"):
            try:
                content = f.read_text(errors="replace")
                if "CertificatePinner" not in content:
                    continue
                patched = re.sub(
                    r"(\.method[^\n]*check[^\n]*\n)(.+?)(\.end method)",
                    r"\1    return-void\n\3",
                    content,
                    flags=re.DOTALL,
                )
                if patched != content:
                    f.write_text(patched)
                    modified.append(str(f))
            except Exception:
                pass

    return _ok(
        modified_files=list(set(modified)),
        patch_summary=f"SSL pinning patched via {strategy} in {len(set(modified))} file(s)",
    )


def _patch_root_detection(source_dir: str, strategy: str = "smali") -> dict:
    d = Path(source_dir)
    if not d.exists():
        return _err("file_not_found", f"Source dir not found: {source_dir}", False)

    modified = []
    if strategy == "smali":
        for f in d.rglob("*.smali"):
            try:
                content = f.read_text(errors="replace")
                if not any(p in content for p in _ROOT_PATTERNS):
                    continue
                # Replace isRooted / checkRoot methods to return false (0)
                patched = re.sub(
                    r"(\.method[^\n]*(?:isRooted|checkRoot|detectRoot)[^\n]*\n)(.+?)(\.end method)",
                    r"\1    const/4 v0, 0x0\n    return v0\n\3",
                    content,
                    flags=re.DOTALL,
                )
                if patched != content:
                    f.write_text(patched)
                    modified.append(str(f))
            except Exception:
                pass

    return _ok(
        modified_files=list(set(modified)),
        patch_summary=f"Root detection patched via {strategy} in {len(set(modified))} file(s)",
    )


def _patch_emulator_detection(source_dir: str, strategy: str = "smali") -> dict:
    d = Path(source_dir)
    if not d.exists():
        return _err("file_not_found", f"Source dir not found: {source_dir}", False)

    modified = []
    if strategy == "smali":
        for f in d.rglob("*.smali"):
            try:
                content = f.read_text(errors="replace")
                if not any(p in content for p in _EMU_PATTERNS):
                    continue
                patched = re.sub(
                    r"(\.method[^\n]*(?:isEmulator|checkEmulator|detectEmulator)[^\n]*\n)(.+?)(\.end method)",
                    r"\1    const/4 v0, 0x0\n    return v0\n\3",
                    content,
                    flags=re.DOTALL,
                )
                if patched != content:
                    f.write_text(patched)
                    modified.append(str(f))
            except Exception:
                pass

    return _ok(
        modified_files=list(set(modified)),
        patch_summary=f"Emulator detection patched via {strategy} in {len(set(modified))} file(s)",
    )

# ---------------------------------------------------------------------------
# Recompile & sign
# ---------------------------------------------------------------------------

def _recompile_apk(source_dir: str, output_apk: str = None) -> dict:
    d = Path(source_dir)
    if not d.exists():
        return _err("file_not_found", f"Source dir not found: {source_dir}", False)

    if not output_apk:
        output_apk = str(d.parent / "patched.apk")

    r = _run(_apktool_cmd() + ["b", str(d), "-o", output_apk])
    if r.returncode != 0:
        return _err("apktool_failure", r.stderr[:500], True, ["check apktool version"])

    size = Path(output_apk).stat().st_size if Path(output_apk).exists() else 0
    return _ok(output_apk=output_apk, size_bytes=size)


def _sign_apk(apk_path: str, output_path: str = None) -> dict:
    apk = Path(apk_path)
    if not apk.exists():
        return _err("file_not_found", f"APK not found: {apk_path}", False)

    if not output_path:
        output_path = str(apk.parent / (apk.stem + "-signed.apk"))

    keystore = _home() / "tools" / "debug.keystore"
    keystore.parent.mkdir(parents=True, exist_ok=True)

    # Generate debug keystore if missing
    if not keystore.exists():
        r = _run([
            _keytool_cmd(), "-genkey", "-v",
            "-keystore", str(keystore),
            "-alias", "androiddebugkey",
            "-keyalg", "RSA", "-keysize", "2048",
            "-validity", "10000",
            "-storepass", "android", "-keypass", "android",
            "-dname", "CN=Android Debug,O=Android,C=US",
        ])
        if r.returncode != 0:
            return _err("keytool_failure", r.stderr[:300], True, ["install java"])

    # Try apksigner first, fall back to jarsigner
    apksigner = _apksigner_or_jarsigner()
    if apksigner:
        r = _run(apksigner + [
            "--ks", str(keystore),
            "--ks-pass", "pass:android",
            "--ks-key-alias", "androiddebugkey",
            "--key-pass", "pass:android",
            "--out", output_path,
            str(apk),
        ])
    else:
        shutil.copy2(str(apk), output_path)
        r = _run([
            "jarsigner", "-verbose",
            "-keystore", str(keystore),
            "-storepass", "android", "-keypass", "android",
            output_path, "androiddebugkey",
        ])

    if r.returncode != 0:
        return _err("sign_failure", r.stderr[:300], True, ["check java/apksigner"])

    size = Path(output_path).stat().st_size if Path(output_path).exists() else 0
    return _ok(output_path=output_path, size_bytes=size, keystore_path=str(keystore))


def _rebuild_and_sign(source_dir: str, output_path: str = None) -> dict:
    recompile = _recompile_apk(source_dir)
    if recompile["status"] != "success":
        return recompile

    unsigned_apk = recompile["output_apk"]
    signed_path = output_path or str(Path(unsigned_apk).parent / "patched-signed.apk")
    sign = _sign_apk(unsigned_apk, signed_path)
    if sign["status"] != "success":
        return sign

    return _ok(
        output_path=sign["output_path"],
        size_bytes=sign["size_bytes"],
        keystore_path=sign["keystore_path"],
    )

# ---------------------------------------------------------------------------
# Interpret findings (LLM or rule-based fallback)
# ---------------------------------------------------------------------------

def _load_llm_config() -> dict:
    cfg_path = _home() / "config.json"
    if not cfg_path.exists():
        return {}
    try:
        cfg = json.loads(cfg_path.read_text())
        return cfg.get("tools", {}).get("llm", {})
    except Exception:
        return {}


def _rule_based_insights(findings: dict) -> dict:
    insights = []
    prot = findings.get("protections", {})
    if prot.get("ssl_pinning"):
        method = prot.get("details", {}).get("ssl_pinning_method", "unknown")
        insights.append(f"SSL pinning detected via {method} — bypass via smali patch or frida hook")
    if prot.get("root_detection"):
        method = prot.get("details", {}).get("root_detection_method", "unknown")
        insights.append(f"Root detection uses {method} — removable via smali edit")
    if prot.get("emulator_detection"):
        insights.append("Emulator detection present — patch Build fields or use real device")
    if prot.get("frida_detection"):
        insights.append("Frida detection present — use frida_spawn with delay_hooks or obfuscated gadget")
    obf = prot.get("obfuscation_level", "low")
    if obf in ("medium", "high"):
        insights.append(f"Obfuscation level: {obf} — jadx decompile recommended for class name recovery")
    strings = findings.get("strings_of_interest", [])
    for s in strings:
        if re.search(r"AES/ECB", s, re.I):
            insights.append("AES/ECB detected — no IV, keys may be statically embedded")
        if re.search(r"http://", s):
            insights.append(f"Plaintext HTTP endpoint found: {s}")
    if not insights:
        insights.append("No critical protections detected — proceed with dynamic analysis")
    return insights


def _call_llm(llm_cfg: dict, prompt: str) -> str:
    provider = llm_cfg.get("provider", "")
    model = llm_cfg.get("model", "")
    api_key = llm_cfg.get("api_key") or os.environ.get(llm_cfg.get("api_key_env", ""), "")

    if provider == "anthropic":
        import urllib.request
        payload = json.dumps({
            "model": model,
            "max_tokens": 1024,
            "messages": [{"role": "user", "content": prompt}],
        }).encode()
        req = urllib.request.Request(
            "https://api.anthropic.com/v1/messages",
            data=payload,
            headers={
                "x-api-key": api_key,
                "anthropic-version": "2023-06-01",
                "content-type": "application/json",
            },
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read())
            return data["content"][0]["text"]

    if provider == "openai":
        import urllib.request
        payload = json.dumps({
            "model": model,
            "messages": [{"role": "user", "content": prompt}],
        }).encode()
        req = urllib.request.Request(
            "https://api.openai.com/v1/chat/completions",
            data=payload,
            headers={"Authorization": f"Bearer {api_key}", "content-type": "application/json"},
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read())
            return data["choices"][0]["message"]["content"]

    if provider == "ollama":
        import urllib.request
        base = llm_cfg.get("api_base", "http://localhost:11434")
        payload = json.dumps({"model": model, "prompt": prompt, "stream": False}).encode()
        req = urllib.request.Request(
            f"{base}/api/generate", data=payload,
            headers={"content-type": "application/json"},
        )
        with urllib.request.urlopen(req, timeout=60) as resp:
            data = json.loads(resp.read())
            return data.get("response", "")

    raise ValueError(f"Unsupported LLM provider: {provider}")


def _interpret_findings(workspace_path: str, focus: str = None) -> dict:
    ws = Path(workspace_path)
    if not ws.exists():
        return _err("file_not_found", f"Workspace not found: {workspace_path}", False)

    # Collect available findings from workspace
    findings = {}
    report_path = ws / "report.json"
    if report_path.exists():
        try:
            findings = json.loads(report_path.read_text())
        except Exception:
            pass

    llm_cfg = _load_llm_config()
    insights_source = "rule-based"
    insights = []
    recommended = []
    risk_summary = ""

    if llm_cfg.get("provider"):
        prompt_data = {
            "workspace": str(ws),
            "findings": findings,
            "focus": focus,
        }
        prompt = (
            "You are a mobile app reverse engineering expert. "
            "Analyze these findings and return a JSON object with keys: "
            "insights (array of strings), recommended_next_steps (array of strings), risk_summary (string).\n\n"
            f"Findings:\n{json.dumps(prompt_data, indent=2)}"
        )
        try:
            raw = _call_llm(llm_cfg, prompt)
            # Extract JSON from response
            match = re.search(r"\{.*\}", raw, re.DOTALL)
            if match:
                parsed = json.loads(match.group())
                insights = parsed.get("insights", [])
                recommended = parsed.get("recommended_next_steps", [])
                risk_summary = parsed.get("risk_summary", "")
                insights_source = "llm"
        except Exception as e:
            insights = _rule_based_insights(findings)
            insights.append(f"(LLM call failed: {e} — falling back to rule-based)")
    else:
        insights = _rule_based_insights(findings)

    return _ok(
        insights=insights,
        recommended_next_steps=recommended,
        risk_summary=risk_summary,
        insights_source=insights_source,
    )


# ---------------------------------------------------------------------------
# run_analysis — meta-tool
# ---------------------------------------------------------------------------

def _run_analysis(target: str, mode: str = "static", focus: str = None) -> dict:
    target_path = Path(target)
    is_apk = target_path.exists() and target_path.suffix.lower() == ".apk"
    name = target_path.stem if is_apk else target.replace(".", "_")
    ws = _workspace(name)

    report = {
        "target": target, "mode": mode, "focus": focus,
        "workspace": str(ws), "findings": {},
        "protections": {}, "patched_apk": None,
    }

    apk_path = str(target_path) if is_apk else None

    # Copy APK to workspace
    if is_apk:
        dest = ws / "original.apk"
        shutil.copy2(str(target_path), str(dest))
        apk_path = str(dest)

    # --- Manifest ---
    if apk_path:
        manifest = _extract_manifest(apk_path)
        if manifest["status"] == "success":
            report["findings"]["permissions"] = manifest.get("permissions", [])
            report["findings"]["manifest"] = manifest

    # --- Decompile ---
    source_dir = None
    if apk_path and mode in ("static", "full", "quick"):
        tool = "both" if mode == "full" else "apktool"
        decompile = _decompile_apk(apk_path, str(ws), tool)
        if decompile["status"] == "success":
            source_dir = decompile["output_dir"]
            report["findings"]["decompile_stats"] = decompile.get("stats", {})

    # --- String search ---
    search_target = source_dir or apk_path or target
    if search_target and mode in ("static", "full", "quick"):
        pattern = "(api_key|secret|password|token|http|AES|RSA)"
        if focus:
            focus_patterns = {
                "crypto": "(AES|DES|RSA|cipher|encrypt|decrypt|key|iv)",
                "network": "(http|https|url|endpoint|api|host)",
                "auth": "(auth|token|login|password|credential|session|jwt)",
                "root-detection": "(root|su|superuser|busybox|magisk)",
            }
            pattern = focus_patterns.get(focus, pattern)
        strings = _search_strings(search_target, pattern, 1)
        if strings["status"] == "success":
            report["findings"]["strings_of_interest"] = [
                m["content"] for m in strings.get("matches", [])[:20]
            ]

    # --- Detect protections ---
    prot_target = source_dir or apk_path
    if prot_target:
        prot = _detect_protections(prot_target)
        if prot["status"] == "success":
            report["protections"] = prot
            report["findings"]["protections"] = prot

    # --- Patch (full mode) ---
    apktool_dir = str(Path(source_dir) / "apktool") if source_dir else None
    if apktool_dir and Path(apktool_dir).exists() and mode == "full":
        prot = report.get("protections", {})
        if prot.get("ssl_pinning"):
            _patch_ssl_pinning(apktool_dir, "smali")
        if prot.get("root_detection"):
            _patch_root_detection(apktool_dir, "smali")
        if prot.get("emulator_detection"):
            _patch_emulator_detection(apktool_dir, "smali")
        rebuild = _rebuild_and_sign(apktool_dir, str(ws / "patched" / "patched-signed.apk"))
        if rebuild["status"] == "success":
            report["patched_apk"] = rebuild["output_path"]

    # --- Save report ---
    report_path = ws / "report.json"
    report_path.write_text(json.dumps(report, indent=2))

    # --- Interpret findings ---
    interp = _interpret_findings(str(ws), focus)

    return _ok(
        workspace=str(ws),
        patched_apk=report.get("patched_apk"),
        findings=report["findings"],
        insights=interp.get("insights", []),
        recommended_next_steps=interp.get("recommended_next_steps", []),
        risk_summary=interp.get("risk_summary", ""),
        insights_source=interp.get("insights_source", "rule-based"),
        confidence=0.8 if interp.get("insights_source") == "llm" else 0.6,
        report_path=str(report_path),
    )

# ---------------------------------------------------------------------------
# MCP tool registry
# ---------------------------------------------------------------------------

@app.tool()
def extract_manifest(apk_path: str) -> str:
    return json.dumps(_extract_manifest(apk_path), indent=2)

@app.tool()
def list_apk_contents(apk_path: str) -> str:
    return json.dumps(_list_apk_contents(apk_path), indent=2)

@app.tool()
def decompile_apk(apk_path: str, output_dir: str = "", tool: str = "both") -> str:
    return json.dumps(_decompile_apk(apk_path, output_dir or None, tool), indent=2)

@app.tool()
def recompile_apk(source_dir: str, output_apk: str = "") -> str:
    return json.dumps(_recompile_apk(source_dir, output_apk or None), indent=2)

@app.tool()
def sign_apk(apk_path: str, output_path: str = "") -> str:
    return json.dumps(_sign_apk(apk_path, output_path or None), indent=2)

@app.tool()
def rebuild_and_sign(source_dir: str, output_path: str = "") -> str:
    return json.dumps(_rebuild_and_sign(source_dir, output_path or None), indent=2)

@app.tool()
def read_smali(file_path: str) -> str:
    return json.dumps(_read_smali(file_path), indent=2)

@app.tool()
def search_strings(target: str, pattern: str, context_lines: int = 2) -> str:
    return json.dumps(_search_strings(target, pattern, context_lines), indent=2)

@app.tool()
def find_class(source_dir: str, class_name: str) -> str:
    return json.dumps(_find_class(source_dir, class_name), indent=2)

@app.tool()
def diff_smali(file_a: str, file_b: str) -> str:
    return json.dumps(_diff_smali(file_a, file_b), indent=2)

@app.tool()
def detect_protections(apk_path_or_dir: str) -> str:
    return json.dumps(_detect_protections(apk_path_or_dir), indent=2)

@app.tool()
def patch_ssl_pinning(source_dir: str, strategy: str = "smali") -> str:
    return json.dumps(_patch_ssl_pinning(source_dir, strategy), indent=2)

@app.tool()
def patch_root_detection(source_dir: str, strategy: str = "smali") -> str:
    return json.dumps(_patch_root_detection(source_dir, strategy), indent=2)

@app.tool()
def patch_emulator_detection(source_dir: str, strategy: str = "smali") -> str:
    return json.dumps(_patch_emulator_detection(source_dir, strategy), indent=2)

@app.tool()
def interpret_findings(workspace_path: str, focus: str = "") -> str:
    return json.dumps(_interpret_findings(workspace_path, focus or None), indent=2)

@app.tool()
def run_analysis(target: str, mode: str = "static", focus: str = "") -> str:
    return json.dumps(_run_analysis(target, mode, focus or None), indent=2)


if __name__ == "__main__":
    app.run()
