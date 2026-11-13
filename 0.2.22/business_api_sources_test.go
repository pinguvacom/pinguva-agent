package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverBusinessAPISourcesFromLocalJSON(t *testing.T) {
	oldOS := runtimeGOOSValue
	runtimeGOOSValue = "linux"
	t.Cleanup(func() { runtimeGOOSValue = oldOS })
	root := t.TempDir()
	oldRoots := businessAPISourceWebRoots
	businessAPISourceWebRoots = []string{root}
	t.Cleanup(func() { businessAPISourceWebRoots = oldRoots })
	logDir := filepath.Join(root, "ilab", "monitoring")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "request.json")
	body := `{
"time":"13.11.2026 10:23:44",
"name":"getPoints",
"method":"GET",
"url":"https://api.example.test/api/consumers/:uuid",
"requestUrl":"https://api.example.test/api/consumers/00000000-0000-4000-8000-000000000001?token=secret",
"httpCode":200,
"totalTime":0.146
}
{
"time":"13.11.2026 10:23:45",
"name":"setIncreasePoints",
"method":"POST",
"url":"https://api.example.test/api/consumers/:uuid/points/increase",
"requestUrl":"https://api.example.test/api/consumers/00000000-0000-4000-8000-000000000001/points/increase",
"httpCode":200,
"totalTime":0.146
}
`
	if err := os.WriteFile(logPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	previews := discoverBusinessAPISources([]agentBusinessAPILogSource{{ID: "source_1", Kind: "web_path", HintPath: "/ilab/monitoring/request.json"}})
	if len(previews) != 1 || previews[0].Status != "ok" || previews[0].ResolvedPath != logPath || len(previews[0].Routes) != 2 {
		t.Fatalf("unexpected source preview: %+v", previews)
	}
	if previews[0].Routes[0].Name != "getPoints" || previews[0].Routes[0].Path != "/api/consumers/:value" {
		t.Fatalf("unexpected discovered route: %+v", previews[0].Routes[0])
	}
	encoded, err := json.Marshal(previews)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "00000000-0000") || strings.Contains(string(encoded), "requestUrl") || strings.Contains(string(encoded), "api.example.test") {
		t.Fatal("private request metadata leaked into preview")
	}
}

func TestDiscoverBusinessAPISourcesMissingAndUnsupported(t *testing.T) {
	oldOS := runtimeGOOSValue
	runtimeGOOSValue = "linux"
	t.Cleanup(func() { runtimeGOOSValue = oldOS })
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "monitoring", "missing.json")
	if got := discoverBusinessAPISources([]agentBusinessAPILogSource{{ID: "missing", Kind: "local_path", HintPath: missing}}); len(got) != 1 || got[0].Status != "missing" {
		t.Fatalf("missing source not reported: %+v", got)
	}
	if err := os.MkdirAll(filepath.Dir(missing), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(missing, []byte(`{"status":true,"message":"sent"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := discoverBusinessAPISources([]agentBusinessAPILogSource{{ID: "unsupported", Kind: "local_path", HintPath: missing}}); len(got) != 1 || got[0].Status != "unsupported_format" {
		t.Fatalf("unsupported source not reported: %+v", got)
	}
}
