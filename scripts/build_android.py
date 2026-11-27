#!/usr/bin/env python3
"""Build the offline Android APK using the installed Android SDK, NDK and Go."""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
from urllib.parse import urlsplit
import zipfile

ROOT = Path(__file__).resolve().parent.parent


def run(arguments, *, env=None):
    print("Running " + Path(str(arguments[0])).name, flush=True)
    subprocess.run([str(arg) for arg in arguments], cwd=ROOT, env=env, check=True)


def latest(directory, pattern):
    candidates = [path for path in directory.glob(pattern) if not path.name.startswith(".")]
    if not candidates:
        raise SystemExit(f"Required development dependency not found in {directory}")
    return max(candidates, key=lambda path: tuple(int(part) for part in re.findall(r"\d+", path.name)))


def default_sdk():
    env_sdk = os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
    if env_sdk:
        return Path(env_sdk)
    if sys.platform == "darwin":
        return Path.home() / "Library/Android/sdk"
    if sys.platform.startswith("linux"):
        return Path.home() / "Android/Sdk"
    return Path.home() / "AppData/Local/Android/Sdk"


def default_ndk(sdk_path):
    env_ndk = os.environ.get("ANDROID_NDK_HOME") or os.environ.get("ANDROID_NDK_ROOT")
    if env_ndk:
        return Path(env_ndk)
    local_windows_ndk = Path("D:/tools/cyledger/android-sdk/ndk/28.2.13676358")
    if local_windows_ndk.is_dir():
        return local_windows_ndk
    ndk_root = sdk_path / "ndk"
    if ndk_root.is_dir():
        return latest(ndk_root, "*")
    return local_windows_ndk


def host_ndk_clang(ndk_path):
    if sys.platform == "win32":
        candidates = ["windows-x86_64/bin/aarch64-linux-android26-clang.cmd"]
    elif sys.platform == "darwin":
        candidates = [
            "darwin-arm64/bin/aarch64-linux-android26-clang",
            "darwin-x86_64/bin/aarch64-linux-android26-clang",
        ]
    else:
        candidates = ["linux-x86_64/bin/aarch64-linux-android26-clang"]
    for rel in candidates:
        candidate = ndk_path / "toolchains/llvm/prebuilt" / rel
        if candidate.is_file():
            return candidate
    return ndk_path / "toolchains/llvm/prebuilt" / candidates[0]


def tool_executable(tools_dir, name):
    suffixes = (".exe", ".bat", ".cmd", "") if sys.platform == "win32" else ("",)
    for suffix in suffixes:
        candidate = tools_dir / f"{name}{suffix}"
        if candidate.is_file():
            return candidate
    raise SystemExit(f"Required Android build-tool '{name}' not found in {tools_dir}")


def resolve_version(tag=None, version_name=None, version_code=None):
    resolved_name = version_name
    resolved_code = version_code
    if tag:
        clean_tag = tag.strip()
        if clean_tag.startswith("v"):
            clean_tag = clean_tag[1:]
        if not resolved_name:
            resolved_name = clean_tag
    if not resolved_name:
        resolved_name = "0.3.0"
    if resolved_code is None:
        if tag or version_name:
            match = re.match(r"^(\d+)\.(\d+)\.(\d+)", resolved_name)
            if not match:
                raise SystemExit(f"Cannot derive versionCode from versionName '{resolved_name}'; pass --version-code")
            major, minor, patch = (int(part) for part in match.groups())
            resolved_code = max(4, major * 10000 + minor * 100 + patch)
        else:
            resolved_code = 4
    if resolved_code < 1:
        raise SystemExit("--version-code must be a positive integer")
    return resolved_name, resolved_code


def market_network_bytes(path):
    config = json.loads(path.read_text(encoding="utf-8")) if path else {}
    if not isinstance(config, dict) or set(config) - {"relayUrl", "relayToken"}:
        raise ValueError("Market network configuration must contain only relayUrl/relayToken")
    address, token = config.get("relayUrl", ""), config.get("relayToken", "")
    if not isinstance(address, str) or not isinstance(token, str):
        raise ValueError("Market relay settings must be strings")
    if address:
        url = urlsplit(address)
        try:
            loopback = ipaddress.ip_address(url.hostname).is_loopback
        except ValueError:
            loopback = False
        if not url.hostname or url.username or url.password or url.path not in ("", "/") or url.query or url.fragment:
            raise ValueError("Use an HTTPS relay origin without path, query or credentials")
        if url.scheme != "https" and not (url.scheme == "http" and loopback):
            raise ValueError("HTTPS is required except for loopback tests")
        if not 24 <= len(token) <= 256 or any(c.isspace() for c in token):
            raise ValueError("Relay token must contain 24-256 non-whitespace characters")
    elif token:
        raise ValueError("Relay token requires a relay URL")
    data = json.dumps(config, ensure_ascii=False).encode("utf-8")
    if len(data) > 4096:
        raise ValueError("Market network configuration is too large")
    return data


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--qa", action="store_true", help="Separate app/data, with WebView debugging")
    parser.add_argument("--skip-frontend", action="store_true", help="Package an already built dist directory")
    parser.add_argument("--skip-backend", action="store_true", help="Reuse already built native libraries")
    parser.add_argument("--sdk", type=Path, default=None, help="Android SDK path")
    parser.add_argument("--ndk", type=Path, default=None, help="Android NDK path")
    parser.add_argument("--tag", help="Release git tag (e.g. v0.3.0) used to derive versionName/versionCode")
    parser.add_argument("--version-name", help="Android versionName (defaults to tag or 0.3.0)")
    parser.add_argument("--version-code", type=int, help="Android versionCode (defaults to tag-derived or 4)")
    parser.add_argument("--require-existing-keystore", action="store_true",
                        help="Fail instead of generating a new keystore when signing files are missing")
    parser.add_argument("--market-network", type=Path, help="Private JSON with relayUrl/relayToken; do not commit it")
    args = parser.parse_args()
    version_name, version_code = resolve_version(args.tag, args.version_name, args.version_code)
    sdk_path = args.sdk or default_sdk()
    ndk_path = args.ndk or default_ndk(sdk_path)
    market_network = market_network_bytes(args.market_network)
    tools = latest(sdk_path / "build-tools", "*")
    platform = latest(sdk_path / "platforms", "android-*") / "android.jar"
    go = shutil.which("go") or "D:/tools/cyledger/go/bin/go.exe"
    java = shutil.which("javac")
    keytool = shutil.which("keytool")
    if not java or not keytool or not platform.is_file():
        raise SystemExit("Java JDK and an Android platform SDK are required")
    compiler = host_ndk_clang(ndk_path)
    if not compiler.is_file():
        raise SystemExit(f"Android NDK clang not found at {compiler}; provide --ndk or ANDROID_NDK_HOME")
    aapt2 = tool_executable(tools, "aapt2")
    d8 = tool_executable(tools, "d8")
    zipalign = tool_executable(tools, "zipalign")
    apksigner = tool_executable(tools, "apksigner")
    npm_cmd = shutil.which("npm.cmd") or shutil.which("npm") or "npm"
    variant = "qa" if args.qa else "release"
    build = ROOT / ".runtime/android" / variant
    native = ROOT / ".runtime/android/native"
    assets = build / "assets"
    resources = build / "res"
    classes = build / "classes"
    dex = build / "dex"
    for folder in (native, assets, resources, classes, dex):
        folder.mkdir(parents=True, exist_ok=True)
    if not args.skip_frontend:
        fe_env = os.environ.copy()
        fe_env["OPENBILL_APP_VERSION"] = version_name
        run([npm_cmd, "run", "build"], env=fe_env)
    if not (ROOT / "dist/mobile.html").is_file():
        raise SystemExit("Build the frontend first (npm run build)")
    if not args.skip_backend:
        env = os.environ.copy()
        env.update(GOOS="android", GOARCH="arm64", CGO_ENABLED="1", CC=str(compiler),
                   CGO_LDFLAGS="-Wl,-z,max-page-size=16384")
        run([go, "build", "-trimpath", "-buildmode=c-shared", f"-ldflags=-s -w -X main.appVersion={version_name}",
             "-o", native / "libcyledger_backend.so", "./android/backend"], env=env)
        run([compiler, "-shared", "-fPIC", "-O2", "-Wl,-z,max-page-size=16384",
             "-Wl,-soname,libcyledger_jni.so", "-I", native,
             ROOT / "android/native/jni.c", "-L", native, "-lcyledger_backend",
             "-o", native / "libcyledger_jni.so"])
    (assets / "market-network.json").write_bytes(market_network)
    digest = hashlib.sha256()
    digest.update(market_network)
    with zipfile.ZipFile(assets / "frontend.zip", "w", zipfile.ZIP_DEFLATED) as archive:
        for path in sorted((ROOT / "dist").rglob("*")):
            if path.is_file() and path.relative_to(ROOT / "dist").parts[0] != "android":
                name = path.relative_to(ROOT / "dist").as_posix()
                data = path.read_bytes()
                digest.update(name.encode())
                digest.update(data)
                archive.writestr(name, data)
    default_config = ROOT / "conf/ezbookkeeping.ini"
    digest.update(default_config.read_bytes())
    (assets / "revision.txt").write_text(digest.hexdigest(), encoding="utf-8")
    shutil.copyfile(default_config, assets / "defaults.ini")
    # License notices travel with the app; no user runtime files enter the APK.
    notices = assets / "licenses"
    notices.mkdir(exist_ok=True)
    for path in [ROOT / "LICENSE", ROOT / "NOTICE", *sorted((ROOT / "licenses").glob("*"))]:
        if path.is_file():
            shutil.copyfile(path, notices / path.name)
    shutil.copytree(ROOT / "android/app/src/main/res", resources, dirs_exist_ok=True)
    (resources / "drawable").mkdir(exist_ok=True)
    # Launcher assets are native adaptive vector resources.
    (resources / "drawable/icon.png").unlink(missing_ok=True)
    manifest_text = (ROOT / "android/app/src/main/AndroidManifest.xml").read_text(encoding="utf-8")
    application_id = "com.cyledger.android.qa" if args.qa else "com.cyledger.android"
    manifest_text = manifest_text.replace("@APPLICATION_ID@", application_id)
    manifest_text = manifest_text.replace("@APP_LABEL@", "OpenBill 测试" if args.qa else "OpenBill")
    manifest_text = manifest_text.replace("@DEBUGGABLE@", "true" if args.qa else "false")
    manifest = build / "AndroidManifest.xml"
    manifest.write_text(manifest_text, encoding="utf-8")
    compiled_resources = build / "resources.zip"
    run([aapt2, "compile", "--dir", resources, "-o", compiled_resources])
    unsigned = build / "unsigned.apk"
    run([aapt2, "link", "-o", unsigned, "--manifest", manifest,
         "-I", platform, "-A", assets, "--version-code", str(version_code), "--version-name", version_name, compiled_resources])
    sources = sorted((ROOT / "android/app/src/main/java").rglob("*.java"))
    bootclasspath = os.pathsep.join([str(tools / "core-lambda-stubs.jar"), str(platform)])
    run([java, "-encoding", "UTF-8", "-source", "8", "-target", "8", "-bootclasspath", bootclasspath,
         "-d", classes, *sources])
    class_files = sorted(classes.rglob("*.class"))
    run([d8, "--min-api", "26", "--lib", platform, "--output", dex, *class_files])
    with zipfile.ZipFile(unsigned, "a", zipfile.ZIP_DEFLATED) as archive:
        for path in sorted(dex.glob("*.dex")):
            archive.write(path, path.name)
        for name in ("libcyledger_backend.so", "libcyledger_jni.so"):
            archive.write(native / name, "lib/arm64-v8a/" + name, compress_type=zipfile.ZIP_STORED)
    aligned = build / "aligned.apk"
    run([zipalign, "-f", "-P", "16", "4", unsigned, aligned])
    signing = ROOT / ".runtime/android-signing"
    signing.mkdir(exist_ok=True)
    password_file = signing / "password.txt"
    keystore = signing / "cyledger.p12"
    if not keystore.exists():
        if args.require_existing_keystore:
            raise SystemExit(f"Existing keystore required but missing: {keystore}")
        password_file.write_text(secrets.token_hex(32), encoding="ascii")
        run([keytool, "-genkeypair", "-keystore", keystore, "-storetype", "PKCS12", "-alias", "cyledger",
             "-storepass:file", password_file, "-keypass:file", password_file,
             "-keyalg", "RSA", "-keysize", "3072", "-validity", "10000", "-dname", "CN=CYLedger Local Build"])
    if not password_file.is_file():
        raise SystemExit("Signing password is missing; preserve the existing key for APK updates")
    output = ROOT / "dist/android"
    output.mkdir(exist_ok=True)
    apk = output / ("OpenBill-Android-arm64-qa.apk" if args.qa else "OpenBill-Android-arm64.apk")
    run([apksigner, "sign", "--ks", keystore, "--ks-pass", "file:" + str(password_file),
         "--ks-key-alias", "cyledger", "--out", apk, aligned])
    run([apksigner, "verify", "--verbose", apk])
    run([zipalign, "-c", "-P", "16", "4", apk])
    result = {"variant": variant, "applicationId": application_id, "versionName": version_name,
              "versionCode": version_code, "apk": str(apk),
              "sha256": hashlib.sha256(apk.read_bytes()).hexdigest(), "bytes": apk.stat().st_size,
              "architecture": "arm64-v8a", "minimumAndroid": "8.0", "targetSdk": 36}
    (build / "build-result.json").write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
