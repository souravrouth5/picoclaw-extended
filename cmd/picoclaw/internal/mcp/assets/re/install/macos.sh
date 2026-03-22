#!/usr/bin/env bash
set -e

echo "==> Installing RE dependencies for macOS"

# Ensure Homebrew is available.
if ! command -v brew &>/dev/null; then
    echo "Homebrew not found. Installing..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
fi

echo "==> Installing java, adb, apktool, jadx via Homebrew..."
brew update
brew install --quiet openjdk apktool jadx android-platform-tools

# Link java if needed.
if ! command -v java &>/dev/null; then
    sudo ln -sfn "$(brew --prefix openjdk)/libexec/openjdk.jdk" \
        /Library/Java/JavaVirtualMachines/openjdk.jdk
fi

echo "==> Installing pip packages..."
pip3 install --upgrade pip
pip3 install mcp frida-tools

echo ""
echo "==> All dependencies installed successfully."
echo "    Connect your Android device via USB with ADB debugging enabled."
echo "    Push frida-server to device: see https://frida.re/docs/android/"
