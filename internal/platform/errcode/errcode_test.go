package errcode

import (
	"errors"
	"strings"
	"testing"
)

// TestServerInternalHidesDetail 系统错误响应不得回显底层错误（表名/SQL/路径）。
func TestServerInternalHidesDetail(t *testing.T) {
	rsp := ServerInternal(errors.New(`dao -> get cluster x: ERROR: relation "clusters" does not exist (SQLSTATE 42P01)`))
	if rsp.Status != CodeServerInternal {
		t.Fatalf("status: %d", rsp.Status)
	}
	if rsp.Message != "internal error" {
		t.Fatalf("message should be generic, got %q", rsp.Message)
	}
	for _, leak := range []string{"SQLSTATE", "clusters", "dao ->"} {
		if strings.Contains(rsp.Message, leak) {
			t.Fatalf("internal detail leaked: %q", rsp.Message)
		}
	}
}
