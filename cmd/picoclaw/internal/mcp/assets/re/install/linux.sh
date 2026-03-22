#!/usr/bin/env bash
set -e

echo "==> Installing RE dependencies for Linux"

# Detect package manager.
if command -v apt-get &>/dev/null; then
    PKG_MGR="apt"
elif command -v dnf &>/dev/null; then
    PKG_MGR="dnf"
elif command -v pacman &>/dev/null; then
    PKG_MGR="pacman"
else
    echo "No supported package manager found (apt/dnf/pacman). Install manually."
    PKG_MGR="none"
fi

echo "==> Installing python3, java, adb..."
case "$PKG_MGR" in
    apt)
        sudo apt-get update -y
        sudo apt-get install -y python3 python3-pip default-jdk adb curl unzip
        ;;
    dnf)
        sudo dnf install -y python3 python3-pip java-17-openjdk android-tools curl unzip
        ;;
    pacman)
        sudo pacman -Sy --noconfirm python python-pip jdk-openjdk android-tools curl unzip
        ;;
esac

echo "==> Installing pip packages..."
pip3 install --upgrade pip
pip3 install mcp frida-tools

echo "==> Installing apktool..."
APKTOOL_VER="2.9.3"
sudo curl -L "https://github.com/iBotPeaches/Apktool/releases/download/v${APKTOOL_VER}/apktool_${APKTOOL_VER}.jar" \
    -o /usr/local/lib/apktool.jar
sudo tee /usr/local/bin/apktool > /dev/null <<'EOF'
#!/usr/bin/env bash
exec java -jar /usr/local/lib/apktool.jar "$@"
EOF
sudo chmod +x /usr/local/bin/apktool

echo "==> Installing jadx..."
JADX_VER="1.5.0"
sudo mkdir -p /opt/jadx
sudo curl -L "https://github.com/skylot/jadx/releases/download/v${JADX_VER}/jadx-${JADX_VER}.zip" \
    -o /tmp/jadx.zip
sudo unzip -o /tmp/jadx.zip -d /opt/jadx
sudo ln -sf /opt/jadx/bin/jadx /usr/local/bin/jadx
rm /tmp/jadx.zip

echo ""
echo "==> All dependencies installed successfully."
echo "    Connect your Android device via USB with ADB debugging enabled."
echo "    Push frida-server to device: see https://frida.re/docs/android/"
