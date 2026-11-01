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
	// 管理面已挂 JWT 中间件：e2e 统一经 dev-token 获取管理员凭证
	t.Setenv("AM_DEV_ISSUE_TOKEN", "1")

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

// adminToken 经 dev-token 端点签发管理员 JWT（AM_ADMIN_GROUP 默认 am:admin）
func adminToken(t *testing.T, rt *platform.Runtime) string {
	t.Helper()
	resp, ar := call(t, rt, http.MethodPost, "/auth/dev-token",
		map[string]any{"sub": "admin-test", "groups": []string{"am:admin"}})
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("dev-token: http=%d body=%+v", resp.StatusCode, ar)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(ar.Data, &issued); err != nil {
		t.Fatalf("decode dev-token: %v", err)
	}
	return issued.Token
}

type apiResponse struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// call 经 httptest 挂载 Engine 发起请求（不占真实端口）；
// bearer 可选携带平台 JWT。
func call(t *testing.T, rt *platform.Runtime, method, path string, body any, bearer ...string) (*http.Response, *apiResponse) {
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
	if len(bearer) > 0 && bearer[0] != "" {
		req.Header.Set("Authorization", "Bearer "+bearer[0])
	}
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)

	resp := rec.Result()
	var ar apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp, &ar
}

// callRaw 不解析响应体（401 等纯文本响应用）
func callRaw(t *testing.T, rt *platform.Runtime, method, path string, bearer ...string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if len(bearer) > 0 && bearer[0] != "" {
		req.Header.Set("Authorization", "Bearer "+bearer[0])
	}
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)
	return rec.Result()
}

func TestClusterCRUDE2E(t *testing.T) {
	rt := newTestRuntime(t)
	tok := adminToken(t, rt)
	id := fmt.Sprintf("it-cluster-%d", time.Now().UnixNano())

	// 未认证 → 401（管理面 JWT 中间件）
	if unauthResp := callRaw(t, rt, http.MethodGet, "/api/v1/clusters"); unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth list: want 401, got %d", unauthResp.StatusCode)
	}

	// 创建
	resp, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": id, "name": "集成测试集群", "desc": "e2e"}, tok)
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("create: http=%d body=%+v", resp.StatusCode, ar)
	}

	// 重复创建 → 业务码 ErrClusterExists
	_, ar = call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": id, "name": "重复"}, tok)
	if ar.Status != errcode.CodeClusterExists {
		t.Fatalf("duplicate create: want code %d, got %+v", errcode.CodeClusterExists, ar)
	}

	// 参数缺失 → ErrInvalidParam
	_, ar = call(t, rt, http.MethodPost, "/api/v1/clusters", map[string]string{"id": ""}, tok)
	if ar.Status != errcode.CodeInvalidParam {
		t.Fatalf("invalid param: want code %d, got %+v", errcode.CodeInvalidParam, ar)
	}

	// 查询
	resp, ar = call(t, rt, http.MethodGet, "/api/v1/clusters/"+id, nil, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("get: http=%d body=%+v", resp.StatusCode, ar)
	}

	// 列表
	_, ar = call(t, rt, http.MethodGet, "/api/v1/clusters", nil, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("list: %+v", ar)
	}

	// 删除
	resp, ar = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+id, nil, tok)
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("delete: http=%d body=%+v", resp.StatusCode, ar)
	}

	// 再查 → ErrClusterNotFound
	_, ar = call(t, rt, http.MethodGet, "/api/v1/clusters/"+id, nil, tok)
	if ar.Status != errcode.CodeClusterNotFound {
		t.Fatalf("get after delete: want code %d, got %+v", errcode.CodeClusterNotFound, ar)
	}
}
