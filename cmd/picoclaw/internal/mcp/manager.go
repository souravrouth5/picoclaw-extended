package mcp

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
)

// KnownServers lists all RE MCP servers in display order.
var KnownServers = []string{"re-static", "re-device", "re-dynamic"}

var serverDescriptions = map[string]string{
	"re-static":  "Static analysis — decompile, patch, recompile, string search",
	"re-device":  "Device interaction — install, pull APK, shell, logcat",
	"re-dynamic": "Dynamic analysis — Frida spawn/attach/capture, built-in hooks",
}

// MCPDir returns the path where MCP server files are extracted.
// ~/.picoclaw/mcp/re/
func MCPDir(picoclawHome string) string {
	return filepath.Join(picoclawHome, "mcp", "re")
}

// ServerPyPath returns the path to the extracted Python server file.
func ServerPyPath(picoclawHome, server string) string {
	return filepath.Join(MCPDir(picoclawHome), server+".py")
}

// ScriptsDirPath returns the path to the extracted Frida scripts directory.
func ScriptsDirPath(picoclawHome string) string {
	return filepath.Join(MCPDir(picoclawHome), "scripts")
}

// ListServers prints all known servers with their enabled status.
func ListServers(cfg *config.Config) {
	fmt.Println("Available MCP servers:")
	fmt.Println()
	for _, name := range KnownServers {
		enabled := isServerEnabled(cfg, name)
		status := "○ disabled"
		if enabled {
			status = "● enabled "
		}
		fmt.Printf("  %s  %-12s  %s\n", status, name, serverDescriptions[name])
	}
	fmt.Println()
	fmt.Println("Use: picoclawx mcp enable <server>")
}

// EnableServer extracts the server, checks deps, optionally installs them,
// then writes the MCP config entry.
func EnableServer(picoclawHome, configPath, server string) error {
	if !isKnownServer(server) {
		return fmt.Errorf("unknown server %q — available: %s", server, strings.Join(KnownServers, ", "))
	}

	fmt.Printf("\nEnabling %s — %s\n\n", server, serverDescriptions[server])

	// Extract server file and scripts.
	if err := extractServer(picoclawHome, server); err != nil {
		return fmt.Errorf("extracting server files: %w", err)
	}

	// Dependency check.
	deps := CheckDeps(server)
	printDeps(deps)

	missing := missingRequired(deps)
	if len(missing) > 0 {
		fmt.Print("Install missing dependencies? (y/n): ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) == "y" {
			if err := runInstallScript(picoclawHome, server); err != nil {
				fmt.Printf("Install script error: %v\n", err)
				fmt.Println("You can install manually and re-run: picoclawx mcp enable " + server)
			} else {
				fmt.Println()
			}
		} else {
			fmt.Println("\nSkipping install. Some tools may not work until deps are installed.")
			fmt.Printf("Run manually: picoclawx mcp install-deps %s\n\n", server)
		}
	}

	// Write config.
	cfg, err := loadRawConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if err := writeServerConfig(cfg, configPath, picoclawHome, server); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("%s enabled. Restart picoclawx to activate.\n\n", server)
	return nil
}

// DisableServer removes the MCP server entry from config.
func DisableServer(configPath, server string) error {
	if !isKnownServer(server) {
		return fmt.Errorf("unknown server %q", server)
	}
	cfg, err := loadRawConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if err := removeServerConfig(cfg, configPath, server); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	fmt.Printf("%s disabled.\n", server)
	return nil
}

// ShowDeps prints the dependency list for a server without enabling it.
func ShowDeps(server string) error {
	if !isKnownServer(server) {
		return fmt.Errorf("unknown server %q", server)
	}
	deps := CheckDeps(server)
	fmt.Printf("\nDependencies for %s:\n\n", server)
	printDeps(deps)
	return nil
}

// InstallDeps extracts and runs the platform install script for a server.
func InstallDeps(picoclawHome, server string) error {
	if !isKnownServer(server) {
		return fmt.Errorf("unknown server %q", server)
	}
	return runInstallScript(picoclawHome, server)
}

// --- internal helpers ---

func isKnownServer(name string) bool {
	for _, s := range KnownServers {
		if s == name {
			return true
		}
	}
	return false
}

func isServerEnabled(cfg *config.Config, server string) bool {
	if cfg.Tools.MCP.Servers == nil {
		return false
	}
	s, ok := cfg.Tools.MCP.Servers[server]
	return ok && s.Enabled
}

func printDeps(deps []Dep) {
	required := []Dep{}
	optional := []Dep{}
	for _, d := range deps {
		if d.Optional {
			optional = append(optional, d)
		} else {
			required = append(required, d)
		}
	}

	fmt.Println("Required:")
	for _, d := range required {
		mark := "✗"
		if d.Found {
			mark = "✓"
		}
		line := fmt.Sprintf("  [%s] %-25s", mark, d.Name)
		if d.Found && d.Version != "" {
			line += "  " + d.Version
		} else if !d.Found && d.Note != "" {
			line += "  " + d.Note
		}
		fmt.Println(line)
	}

	if len(optional) > 0 {
		fmt.Println("\nOptional:")
		for _, d := range optional {
			mark := "✗"
			if d.Found {
				mark = "✓"
			}
			line := fmt.Sprintf("  [%s] %-25s", mark, d.Name)
			if d.Note != "" {
				line += "  " + d.Note
			}
			fmt.Println(line)
		}
	}
	fmt.Println()
}

func missingRequired(deps []Dep) []Dep {
	var missing []Dep
	for _, d := range deps {
		if !d.Optional && !d.Found {
			missing = append(missing, d)
		}
	}
	return missing
}

// extractServer copies the Python server file (and scripts for re-dynamic)
// from the embedded FS to ~/.picoclaw/mcp/re/.
func extractServer(picoclawHome, server string) error {
	destDir := MCPDir(picoclawHome)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	// Extract the server .py file.
	srcPath := "assets/re/" + server + ".py"
	data, err := Assets.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("embedded file not found: %s", srcPath)
	}
	destPath := filepath.Join(destDir, server+".py")
	if err := os.WriteFile(destPath, data, 0o755); err != nil {
		return err
	}
	fmt.Printf("  extracted: %s\n", destPath)

	// For re-dynamic, also extract all Frida scripts.
	if server == "re-dynamic" {
		scriptsDir := ScriptsDirPath(picoclawHome)
		if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
			return err
		}
		err := fs.WalkDir(Assets, "assets/re/scripts", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := Assets.ReadFile(path)
			if err != nil {
				return err
			}
			dest := filepath.Join(scriptsDir, filepath.Base(path))
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				return err
			}
			fmt.Printf("  extracted: %s\n", dest)
			return nil
		})
		if err != nil {
			return fmt.Errorf("extracting scripts: %w", err)
		}
	}

	// Extract the platform install script.
	platform := DetectPlatform()
	installScript := installScriptName(platform)
	installSrc := "assets/re/install/" + installScript
	installData, err := Assets.ReadFile(installSrc)
	if err == nil {
		installDest := filepath.Join(destDir, "install-"+server+"-"+installScript)
		_ = os.WriteFile(installDest, installData, 0o755)
	}

	return nil
}

func installScriptName(p Platform) string {
	switch p {
	case PlatformWindows:
		return "windows.bat"
	case PlatformMacOS:
		return "macos.sh"
	case PlatformTermux:
		return "termux.sh"
	default:
		return "linux.sh"
	}
}

// runInstallScript runs the platform-specific install script for a server.
func runInstallScript(picoclawHome, server string) error {
	platform := DetectPlatform()
	scriptName := "install-" + server + "-" + installScriptName(platform)
	scriptPath := filepath.Join(MCPDir(picoclawHome), scriptName)

	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		// Re-extract in case it wasn't extracted yet.
		if err := extractServer(picoclawHome, server); err != nil {
			return err
		}
	}

	fmt.Printf("Running install script: %s\n\n", scriptPath)

	var cmd *exec.Cmd
	if platform == PlatformWindows {
		cmd = exec.Command("cmd", "/C", scriptPath)
	} else {
		cmd = exec.Command("bash", scriptPath)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// writeServerConfig patches config.json to add the MCP server entry.
// Uses raw JSON manipulation to avoid clobbering unrelated config fields.
func writeServerConfig(raw map[string]json.RawMessage, configPath, picoclawHome, server string) error {
	pyPath := ServerPyPath(picoclawHome, server)

	// Resolve python binary name.
	py := "python3"
	if runtime.GOOS == "windows" {
		py = "python"
	}

	serverEntry := map[string]interface{}{
		"enabled": true,
		"command": py,
		"args":    []string{pyPath},
		"type":    "stdio",
	}
	serverEntryJSON, _ := json.Marshal(serverEntry)

	// Drill into tools.mcp.servers.
	tools := getRawObject(raw, "tools")
	mcp := getRawObject(tools, "mcp")

	// Force mcp.enabled = true.
	mcp["enabled"] = json.RawMessage(`true`)

	// Get or create servers map.
	servers := getRawObject(mcp, "servers")
	servers[server] = serverEntryJSON

	// Write back up the chain.
	serversJSON, _ := json.Marshal(servers)
	mcp["servers"] = serversJSON
	mcpJSON, _ := json.Marshal(mcp)
	tools["mcp"] = mcpJSON
	toolsJSON, _ := json.Marshal(tools)
	raw["tools"] = toolsJSON

	return writeRawConfig(configPath, raw)
}

// removeServerConfig removes a server entry from tools.mcp.servers.
func removeServerConfig(raw map[string]json.RawMessage, configPath, server string) error {
	tools := getRawObject(raw, "tools")
	mcp := getRawObject(tools, "mcp")
	servers := getRawObject(mcp, "servers")

	delete(servers, server)

	serversJSON, _ := json.Marshal(servers)
	mcp["servers"] = serversJSON
	mcpJSON, _ := json.Marshal(mcp)
	tools["mcp"] = mcpJSON
	toolsJSON, _ := json.Marshal(tools)
	raw["tools"] = toolsJSON

	return writeRawConfig(configPath, raw)
}

// getRawObject reads a key from a raw JSON map as a nested object map.
// Returns an empty map if the key is missing or not an object.
func getRawObject(parent map[string]json.RawMessage, key string) map[string]json.RawMessage {
	result := map[string]json.RawMessage{}
	if v, ok := parent[key]; ok {
		_ = json.Unmarshal(v, &result)
	}
	return result
}

func loadRawConfig(configPath string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func writeRawConfig(configPath string, raw map[string]json.RawMessage) error {
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0o600)
}
