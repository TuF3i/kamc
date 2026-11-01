package deploy

// 部署产物语法守护：manifests 变更必须仍是可解析的多文档 YAML。
// （占位符以 __NAME__ 形式存在，解析前替换为安全值。）

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func manifestFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, dir := range []string{"platform", "agent"} {
		path := filepath.Join(dir, "manifests.yaml")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("manifest missing: %v", err)
		}
		files = append(files, path)
	}
	return files
}

func TestManifestsAreValidYAML(t *testing.T) {
	for _, file := range manifestFiles(t) {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		// 占位符值形如 __X__，替换为字符串避免 YAML 解析歧义
		content := strings.ReplaceAll(string(raw), "__", "PH_")
		docs := strings.Split(content, "\n---")
		for i, doc := range docs {
			if strings.TrimSpace(doc) == "" {
				continue
			}
			var out map[string]any
			if err := yaml.Unmarshal([]byte(doc), &out); err != nil {
				t.Fatalf("%s doc#%d: %v", file, i+1, err)
			}
		}
	}
}

// TestPlatformManifestEssentials 平台清单关键结构抽查（HA 语义不可回退）。
func TestPlatformManifestEssentials(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("platform", "manifests.yaml"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(raw)
	for _, want := range []string{
		"replicas: 3",
		"minAvailable: 2",
		"clusterIP: None",
		"k8s-am-platform-headless",
		"terminationGracePeriodSeconds: 60",
		"PEER_ADDRESSES",
		"PEER_TOKEN",
		"JWT_KEYRING_DIR",
		"NetworkPolicy",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("platform manifest missing %q", want)
		}
	}
}

// TestAgentManifestEssentials Agent 清单关键结构抽查。
func TestAgentManifestEssentials(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("agent", "manifests.yaml"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(raw)
	for _, want := range []string{
		"replicas: 2",
		"cluster-admin",
		"HEARTBEAT_PING_INTERVAL",
		"podAntiAffinity",
		"TOKEN_FILE",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("agent manifest missing %q", want)
		}
	}
}
