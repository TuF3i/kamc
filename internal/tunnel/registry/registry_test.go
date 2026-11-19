package registry

import "testing"

// TestValidateID 隧道标识白名单：拒绝 Redis glob 元字符与键分隔符，
// 防止 Lookup 的 SCAN 模式被注入（clusterID 来自外部输入）。
func TestValidateID(t *testing.T) {
	valid := []string{"prod-bj", "c1", "a.b_c-1", "A1", "x"}
	for _, v := range valid {
		if err := validateID("cluster id", v); err != nil {
			t.Fatalf("valid id %q rejected: %v", v, err)
		}
	}
	invalid := []string{"", "*", "a*", "a?b", "a[b]", "a:b", "../x", "a b", "-lead", ".lead"}
	for _, v := range invalid {
		if err := validateID("cluster id", v); err == nil {
			t.Fatalf("invalid id %q accepted", v)
		}
	}
}
