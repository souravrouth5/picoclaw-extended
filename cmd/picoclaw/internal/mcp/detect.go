package mcp

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Platform represents the detected runtime environment.
type Platform int

const (
	PlatformTermux  Platform = iota // Android + Termux
	PlatformLinux                   // Linux desktop/server
	PlatformMacOS                   // macOS
	PlatformWindows                 // Windows
)

func (p Platform) String() string {
	switch p {
	case PlatformTermux:
		return "termux"
	case PlatformLinux:
		return "linux"
	case PlatformMacOS:
		return "macos"
	case PlatformWindows:
		return "windows"
	default:
		return "unknown"
	}
}

// DetectPlatform returns the current platform.
func DetectPlatform() Platform {
	if prefix := os.Getenv("PREFIX"); strings.Contains(prefix, "com.termux") {
		return PlatformTermux
	}
	switch runtime.GOOS {
	case "darwin":
		return PlatformMacOS
	case "windows":
		return PlatformWindows
	default:
		return PlatformLinux
	}
}

// Dep represents a single dependency with its check result.
type Dep struct {
	Name     string
	Optional bool
	Found    bool
	Version  string // populated when found
	Note     string // extra context shown to user
}

// CheckDeps checks all dependencies for the given server name.
// Returns required deps first, then optional.
func CheckDeps(server string) []Dep {
	switch server {
	case "re-static":
		return checkDepsStatic()
	case "re-device":
		return checkDepsDevice()
	case "re-dynamic":
		return checkDepsDynamic()
	default:
		return nil
	}
}

func checkDepsStatic() []Dep {
	return []Dep{
		checkBinary("python3", false, ""),
		checkBinary("pip3", false, ""),
		checkPipPackage("mcp", false, ""),
		checkBinary("apktool", false, "requires Java 8+"),
		checkBinary("java", false, "required by apktool"),
		checkBinary("jadx", true, "optional but produces better decompile output"),
		checkBinary("keytool", true, "for APK signing; usually bundled with Java"),
	}
}

func checkDepsDevice() []Dep {
	platform := DetectPlatform()
	adbNote := ""
	if platform == PlatformTermux {
		adbNote = "on Termux: pkg install android-tools"
	}
	return []Dep{
		checkBinary("python3", false, ""),
		checkBinary("pip3", false, ""),
		checkPipPackage("mcp", false, ""),
		checkBinary("adb", false, adbNote),
	}
}

func checkDepsDynamic() []Dep {
	platform := DetectPlatform()
	adbNote := ""
	if platform == PlatformTermux {
		adbNote = "on Termux: pkg install android-tools"
	}
	return []Dep{
		checkBinary("python3", false, ""),
		checkBinary("pip3", false, ""),
		checkPipPackage("mcp", false, ""),
		checkPipPackage("frida-tools", false, "pip3 install frida-tools"),
		checkBinary("adb", false, adbNote),
		{
			Name:     "frida-server",
			Optional: true,
			Found:    checkFridaServer(),
			Note:     "on-device binary; install script downloads correct arch automatically",
		},
	}
}

// checkBinary checks whether a binary exists in PATH and captures its version.
func checkBinary(name string, optional bool, note string) Dep {
	d := Dep{Name: name, Optional: optional, Note: note}
	path, err := exec.LookPath(name)
	if err != nil {
		return d
	}
	d.Found = true
	// Try to get version via --version flag; ignore errors.
	out, err := exec.Command(path, "--version").Output()
	if err == nil {
		line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
		if len(line) > 60 {
			line = line[:60]
		}
		d.Version = line
	}
	return d
}

// checkPipPackage checks whether a pip package is importable.
func checkPipPackage(pkg string, optional bool, note string) Dep {
	d := Dep{Name: pkg + " (pip)", Optional: optional, Note: note}
	py := "python3"
	if runtime.GOOS == "windows" {
		py = "python"
	}
	err := exec.Command(py, "-c", "import "+strings.ReplaceAll(pkg, "-", "_")).Run()
	d.Found = err == nil
	return d
}

// checkFridaServer checks for frida-server in /data/local/tmp (Termux/on-device).
func checkFridaServer() bool {
	_, err := os.Stat("/data/local/tmp/frida-server")
	return err == nil
}
