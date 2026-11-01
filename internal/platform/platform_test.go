package platform_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/TuF3i/kamc/infrastructure/postgres"
	"github.com/TuF3i/kamc/internal/platform"
	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// e2e 走完整 DI 链：数据库用例按约定由 TEST_PG_DSN 门控（数据库不进普通单测）。
func newTestRuntime(t *testing.T) *platform.Runtime {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skip database e2e")
	}
	// db 由测试自建并注入（ownDB=false），释放由测试负责
	db, err := postgres.NewPostgresClient(postgres.WithDSN(dsn))
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	rt, err := platform.CreateRuntime(platform.WithDB(db), platform.WithAddr(":0"))
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	t.Cleanup(func() { _ = rt.ReleaseRuntime() })
	return rt
}

type apiResponse struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// call 经 httptest 挂载 Engine 发起请求（不占真实端口）
func call(t *testing.T, rt *platform.Runtime, method, path string, body any) (*http.Response, *apiResponse) {
	t.Helper()
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)

	resp := rec.Result()
	var ar apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp, &ar
}

func TestClusterCRUDE2E(t *testing.T) {
	rt := newTestRuntime(t)
	id := fmt.Sprintf("it-cluster-%d", time.Now().UnixNano())

	// 创建
	resp, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": id, "name": "集成测试集群", "desc": "e2e"})
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("create: http=%d body=%+v", resp.StatusCode, ar)
	}

	// 重复创建 → 业务码 ErrClusterExists
	_, ar = call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": id, "name": "重复"})
	if ar.Status != errcode.CodeClusterExists {
		t.Fatalf("duplicate create: want code %d, got %+v", errcode.CodeClusterExists, ar)
	}

	// 参数缺失 → ErrInvalidParam
	_, ar = call(t, rt, http.MethodPost, "/api/v1/clusters", map[string]string{"id": ""})
	if ar.Status != errcode.CodeInvalidParam {
		t.Fatalf("invalid param: want code %d, got %+v", errcode.CodeInvalidParam, ar)
	}

	// 查询
	resp, ar = call(t, rt, http.MethodGet, "/api/v1/clusters/"+id, nil)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("get: http=%d body=%+v", resp.StatusCode, ar)
	}

	// 列表
	_, ar = call(t, rt, http.MethodGet, "/api/v1/clusters", nil)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("list: %+v", ar)
	}

	// 删除
	resp, ar = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+id, nil)
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("delete: http=%d body=%+v", resp.StatusCode, ar)
	}

	// 再查 → ErrClusterNotFound
	_, ar = call(t, rt, http.MethodGet, "/api/v1/clusters/"+id, nil)
	if ar.Status != errcode.CodeClusterNotFound {
		t.Fatalf("get after delete: want code %d, got %+v", errcode.CodeClusterNotFound, ar)
	}
}
