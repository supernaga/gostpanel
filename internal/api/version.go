package api

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
)

// AgentVersion represents the agent version info.
type AgentVersion struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Checksum  string `json:"checksum,omitempty"`
}

// CurrentAgentVersion is the current agent version.
// This can be overridden via ldflags at build time:
// go build -ldflags "-X github.com/AliceNetworks/gost-panel/internal/api.CurrentAgentVersion=1.4.0"
var CurrentAgentVersion = "dev"
var AgentBuildTime = "unknown"

var supportedAgentTargets = map[string]map[string]bool{
	"linux": {
		"amd64":    true,
		"arm64":    true,
		"386":      true,
		"arm":      true,
		"armv7":    true,
		"armv6":    true,
		"armv5":    true,
		"mips":     true,
		"mipsle":   true,
		"mips64":   true,
		"mips64le": true,
	},
	"darwin": {
		"amd64": true,
		"arm64": true,
	},
	"windows": {
		"amd64": true,
		"arm64": true,
		"386":   true,
	},
	"freebsd": {
		"amd64": true,
		"arm64": true,
	},
}

// agentGetVersion returns the current agent version.
func (s *Server) agentGetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, AgentVersion{
		Version:   CurrentAgentVersion,
		BuildTime: AgentBuildTime,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	})
}

// agentCheckUpdate checks if an update is available.
func (s *Server) agentCheckUpdate(c *gin.Context) {
	clientVersion := c.Query("version")
	clientOS := normalizeAgentOS(c.DefaultQuery("os", runtime.GOOS))
	clientArch := normalizeAgentArch(c.DefaultQuery("arch", runtime.GOARCH))

	needsUpdate := compareVersions(clientVersion, CurrentAgentVersion) < 0

	response := gin.H{
		"current_version": clientVersion,
		"latest_version":  CurrentAgentVersion,
		"needs_update":    needsUpdate,
		"build_time":      AgentBuildTime,
	}

	if needsUpdate && isSupportedAgentTarget(clientOS, clientArch) {
		if binaryPath, _, ok := findAgentBinary(clientOS, clientArch); ok {
			checksum, _ := getFileChecksum(binaryPath)
			response["download_url"] = fmt.Sprintf("/agent/download/%s/%s", clientOS, clientArch)
			response["checksum"] = checksum
		}
	}

	c.JSON(http.StatusOK, response)
}

// agentDownload serves the agent binary for download.
func (s *Server) agentDownload(c *gin.Context) {
	osName := normalizeAgentOS(c.Param("os"))
	archName := normalizeAgentArch(c.Param("arch"))

	if !isSupportedAgentTarget(osName, archName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid os or arch"})
		return
	}

	binaryPath, fileName, ok := findAgentBinary(osName, archName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "agent binary not found",
			"message": fmt.Sprintf("Binary for %s/%s is not available. Please build it first.", osName, archName),
		})
		return
	}

	fileInfo, err := os.Stat(binaryPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	c.Header("X-Agent-Version", CurrentAgentVersion)

	if checksum, err := getFileChecksum(binaryPath); err == nil {
		c.Header("X-Checksum-SHA256", checksum)
	}

	c.File(binaryPath)
}

func normalizeAgentOS(osName string) string {
	switch strings.ToLower(strings.TrimSpace(osName)) {
	case "macos", "osx":
		return "darwin"
	default:
		return strings.ToLower(strings.TrimSpace(osName))
	}
}

func normalizeAgentArch(archName string) string {
	switch strings.ToLower(strings.TrimSpace(archName)) {
	case "x86_64", "x64":
		return "amd64"
	case "aarch64":
		return "arm64"
	case "i386", "i686", "x86":
		return "386"
	case "armv7l":
		return "armv7"
	case "armv6l":
		return "armv6"
	default:
		return strings.ToLower(strings.TrimSpace(archName))
	}
}

func isSupportedAgentTarget(osName, archName string) bool {
	arches, ok := supportedAgentTargets[osName]
	if !ok {
		return false
	}
	return arches[archName]
}

func findAgentBinary(osName, archName string) (string, string, bool) {
	fileCandidates := agentFileCandidates(osName, archName)

	for _, dir := range agentSearchDirs() {
		for _, fileName := range fileCandidates {
			path := filepath.Join(dir, fileName)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, fileName, true
			}
		}
	}

	return "", "", false
}

func agentFileCandidates(osName, archName string) []string {
	buildName := func(arch string) string {
		name := fmt.Sprintf("gost-agent-%s-%s", osName, arch)
		if osName == "windows" {
			name += ".exe"
		}
		return name
	}

	candidates := []string{buildName(archName)}

	if osName == "linux" {
		switch archName {
		case "arm":
			candidates = append(candidates, buildName("armv7"), buildName("armv6"), buildName("armv5"))
		case "armv7", "armv6", "armv5":
			candidates = append(candidates, buildName("arm"))
		}
	}

	return uniqueStrings(candidates)
}

func agentSearchDirs() []string {
	dirs := make([]string, 0, 8)

	if custom := strings.TrimSpace(os.Getenv("AGENT_DIST_DIR")); custom != "" {
		dirs = append(dirs, custom)
	}

	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		dirs = append(dirs, filepath.Join(execDir, "dist", "agents"))
		dirs = append(dirs, filepath.Join(execDir, "agents"))
	}

	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(wd, "dist", "agents"))
	}

	dirs = append(dirs,
		"/opt/gost-panel/dist/agents",
		"/opt/gost-panel/agents",
	)

	return uniqueStrings(dirs)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		value = filepath.Clean(value)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}

// getFileChecksum calculates SHA256 checksum of a file.
func getFileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// compareVersions compares two semantic versions.
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
func compareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &n1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &n2)
		}

		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	return 0
}

// clientHeartbeat handles client heartbeat requests.
func (s *Server) clientHeartbeat(c *gin.Context) {
	token := c.Param("token")

	err := s.svc.UpdateClientHeartbeat(token)
	if err != nil {
		c.JSON(http.StatusGone, gin.H{"error": "client not found", "uninstall": true})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
