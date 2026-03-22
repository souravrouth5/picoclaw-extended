#!/data/data/com.termux/files/usr/bin/bash
set -e

echo "==> Installing RE dependencies for Termux"

echo "==> Updating package list..."
pkg update -y

echo "==> Installing python, java, adb..."
pkg install -y python openjdk-17 android-tools

echo "==> Installing pip packages..."
pip3 install --upgrade pip
pip3 install mcp frida-tools

echo "==> Installing apktool..."
pkg install -y apktool 2>/dev/null || {
    echo "apktool not in pkg — downloading jar..."
    APKTOOL_VER="2.9.3"
    APKTOOL_JAR="$PREFIX/bin/apktool.jar"
    APKTOOL_WRAPPER="$PREFIX/bin/apktool"
    curl -L "https://github.com/iBotPeaches/Apktool/releases/download/v${APKTOOL_VER}/apktool_${APKTOOL_VER}.jar" -o "$APKTOOL_JAR"
    cat > "$APKTOOL_WRAPPER" <<'EOF'
#!/data/data/com.termux/files/usr/bin/bash
exec java -jar "$PREFIX/bin/apktool.jar" "$@"
EOF
    chmod +x "$APKTOOL_WRAPPER"
}

echo "==> Installing jadx..."
JADX_VER="1.5.0"
JADX_DIR="$PREFIX/opt/jadx"
mkdir -p "$JADX_DIR"
curl -L "https://github.com/skylot/jadx/releases/download/v${JADX_VER}/jadx-${JADX_VER}.zip" -o /tmp/jadx.zip
unzip -o /tmp/jadx.zip -d "$JADX_DIR"
ln -sf "$JADX_DIR/bin/jadx" "$PREFIX/bin/jadx"
rm /tmp/jadx.zip

echo "==> Downloading frida-server for device arch..."
ARCH=$(uname -m)
case "$ARCH" in
    aarch64) FRIDA_ARCH="arm64" ;;
    armv7l|armv8l) FRIDA_ARCH="arm" ;;
    x86_64) FRIDA_ARCH="x86_64" ;;
    i686) FRIDA_ARCH="x86" ;;
    *) echo "Unknown arch: $ARCH — skipping frida-server download"; exit 0 ;;
esac

FRIDA_VER=$(pip3 show frida 2>/dev/null | grep Version | awk '{print $2}')
if [ -z "$FRIDA_VER" ]; then
    FRIDA_VER="16.2.1"
fi

FRIDA_URL="https://github.com/frida/frida/releases/download/${FRIDA_VER}/frida-server-${FRIDA_VER}-android-${FRIDA_ARCH}.xz"
echo "==> Downloading frida-server ${FRIDA_VER} for ${FRIDA_ARCH}..."
curl -L "$FRIDA_URL" -o /tmp/frida-server.xz
xz -d /tmp/frida-server.xz
mv /tmp/frida-server /data/local/tmp/frida-server
chmod 755 /data/local/tmp/frida-server

echo ""
echo "==> All dependencies installed successfully."
echo "    frida-server is at /data/local/tmp/frida-server"
echo "    Start it with: su -c '/data/local/tmp/frida-server &'"
