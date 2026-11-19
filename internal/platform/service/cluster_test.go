package service

import "testing"

// TestValidClusterID 创建集群时的字符集约束：ID 会进入 URL/kubeconfig/Redis 键，
// 必须拒绝 glob 元字符、路径分隔符与空白。
func TestValidClusterID(t *testing.T) {
	for _, id := range []string{"prod-bj", "c1", "a.b_c-1", "A1", "sre-test"} {
		if !validClusterID(id) {
			t.Fatalf("valid id %q rejected", id)
		}
	}
	for _, id := range []string{"", "*", "a*", "a?b", "a:b", "../x", "a b", "-lead", ".lead", "中文"} {
		if validClusterID(id) {
			t.Fatalf("invalid id %q accepted", id)
		}
	}
}
