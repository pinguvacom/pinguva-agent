package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	businessAPISourceMaxCount  = 8
	businessAPISourceMaxRoutes = 100
	businessAPISourceMaxBytes  = int64(4 << 20)
)

var businessAPISafeSourceNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

type agentBusinessAPILogSource struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	HintPath string `json:"hintPath"`
}

type agentBusinessAPISourceRoute struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

type agentBusinessAPISourcePreview struct {
	ID           string                        `json:"id"`
	Status       string                        `json:"status"`
	ResolvedPath string                        `json:"resolvedPath,omitempty"`
	Routes       []agentBusinessAPISourceRoute `json:"routes,omitempty"`
}

var businessAPISourceWebRoots = []string{
	"/home/bitrix/www",
	"/var/www/html",
	"/srv/www",
	"/usr/share/nginx/html",
}

func businessAPISourceHintSafe(value string) bool {
	value = normalizeAgentBusinessAPILogPath(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	base := filepath.Base(lower)
	return (strings.Contains(lower, "/monitoring/") || strings.Contains(lower, "/log/") || strings.Contains(lower, "/logs/")) &&
		(strings.HasSuffix(base, ".json") || strings.HasSuffix(base, ".log") || strings.HasSuffix(base, ".txt"))
}

func syncBusinessAPILogSources(state *agentState, response *agentReportResponse) bool {
	if state == nil || response == nil || response.BusinessAPILogSources == nil {
		return false
	}
	next := make([]agentBusinessAPILogSource, 0, min(len(*response.BusinessAPILogSources), businessAPISourceMaxCount))
	for _, source := range *response.BusinessAPILogSources {
		if source.ID == "" || (source.Kind != "web_path" && source.Kind != "local_path") || !businessAPISourceHintSafe(source.HintPath) {
			continue
		}
		source.HintPath = normalizeAgentBusinessAPILogPath(source.HintPath)
		next = append(next, source)
		if len(next) >= businessAPISourceMaxCount {
			break
		}
	}
	if slices.Equal(state.BusinessAPILogSources, next) {
		return false
	}
	state.BusinessAPILogSources = next
	return true
}

func businessAPISourceLocalFile(source agentBusinessAPILogSource) (string, string) {
	if !businessAPISourceHintSafe(source.HintPath) {
		return "", "invalid_path"
	}
	paths := []string{source.HintPath}
	if source.Kind == "web_path" {
		paths = nil
		for _, root := range businessAPISourceWebRoots {
			paths = append(paths, filepath.Join(root, strings.TrimPrefix(source.HintPath, "/")))
		}
	}
	status := "missing"
	for _, candidate := range paths {
		info, err := os.Lstat(candidate)
		if os.IsPermission(err) {
			status = "permission_denied"
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		resolvedFile, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr != nil {
			continue
		}
		if source.Kind == "web_path" {
			root := strings.TrimSuffix(candidate, source.HintPath)
			resolvedRoot, rootErr := filepath.EvalSymlinks(root)
			if rootErr != nil || !strings.HasPrefix(resolvedFile, resolvedRoot+string(os.PathSeparator)) {
				continue
			}
		} else if resolvedFile != candidate {
			continue
		}
		file, err := os.Open(candidate)
		if os.IsPermission(err) {
			status = "permission_denied"
			continue
		}
		if err != nil {
			continue
		}
		file.Close()
		return candidate, "ok"
	}
	return "", status
}

func discoverBusinessAPISources(sources []agentBusinessAPILogSource) []agentBusinessAPISourcePreview {
	if len(sources) == 0 || runtimeGOOS() != "linux" {
		return nil
	}
	previews := make([]agentBusinessAPISourcePreview, 0, min(len(sources), businessAPISourceMaxCount))
	for _, source := range sources[:min(len(sources), businessAPISourceMaxCount)] {
		preview := agentBusinessAPISourcePreview{ID: source.ID, Status: "missing"}
		path, status := businessAPISourceLocalFile(source)
		preview.Status = status
		if status != "ok" {
			previews = append(previews, preview)
			continue
		}
		preview.ResolvedPath = path
		lines, err := readBusinessAPILogTail(path, businessAPISourceMaxBytes)
		if err != nil {
			preview.Status = businessAPILogReadError(err)
			previews = append(previews, preview)
			continue
		}
		seen := make(map[string]struct{})
		for _, item := range parseBusinessAPILogLines(lines) {
			if item.Name == "" || item.Method == "" || item.Path == "" || len(item.Path) > 512 {
				continue
			}
			key := item.Method + "|" + item.Path
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			preview.Routes = append(preview.Routes, agentBusinessAPISourceRoute{Name: item.Name, Method: item.Method, Path: item.Path})
			if len(preview.Routes) >= businessAPISourceMaxRoutes {
				break
			}
		}
		if len(lines) > 0 && len(preview.Routes) == 0 {
			preview.Status = "unsupported_format"
		}
		previews = append(previews, preview)
	}
	return previews
}
