// Package errcode 统一响应与业务错误码（错误处理模式 B：Response 即 error）。
//
// 约定：
//   - 业务错误 = 本包包级 var（不可变共享，判定用 errors.Is 指针相等），不是构造调用；
//   - 系统错误 = ServerInternal(err) 构造，携带底层详情，仅在出口层使用；
//   - 层间方法签名 (data, rsp) 元组，错误原样上抛不包装；
//   - handler 统一出口 GenFinalResponse：HTTP 恒 200（系统错误 500），业务码在 status 字段。
//
// 本包是叶包：不 import 项目内任何包。
package errcode

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// 业务码段规划：0 成功；1xxxx 通用；2xxxx 集群域；3xxxx Token 域；4xxxx 策略域；5xxxx 系统（≥50000 走 HTTP 500）
const (
	CodeSuccess            = 0
	CodeInvalidParam       = 10001
	CodeClusterNotFound    = 20001
	CodeClusterExists      = 20002
	CodeClusterUnreachable = 20003
	CodeDiscoveryFailed    = 20004
	CodeTokenNotFound      = 30001
	CodeRobotExists        = 30002
	CodeRobotNotFound      = 30003
	CodeRobotExpired       = 30004
	CodePolicyNotFound     = 40001
	CodeUserNotFound       = 40006
	CodeUserExists         = 40010
	CodeGroupExists        = 40007
	CodeGroupNotFound      = 40008
	CodeLDAPDisabled       = 10002
	CodeScriptsDisabled    = 10004
	CodeScriptFailed       = 10003
	CodeAuthScriptNotFound = 40009
	CodeServerInternal     = 50000
)

// Response 统一响应体；实现 error 接口供层间元组上抛。
type Response struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// Error 实现 error 接口（仅用于 errors.Is 判定与日志，不直接写响应）
func (r *Response) Error() string {
	return fmt.Sprintf("errcode(%d): %s", r.Status, r.Message)
}

// 业务错误码（包级 var，禁止运行期修改）
var (
	OperationSuccess      = &Response{Status: CodeSuccess, Message: "success"}
	ErrInvalidParam       = &Response{Status: CodeInvalidParam, Message: "invalid parameter"}
	ErrClusterNotFound    = &Response{Status: CodeClusterNotFound, Message: "cluster not found"}
	ErrClusterExists      = &Response{Status: CodeClusterExists, Message: "cluster already exists"}
	ErrClusterUnreachable = &Response{Status: CodeClusterUnreachable, Message: "cluster unreachable"}
	ErrDiscoveryFailed    = &Response{Status: CodeDiscoveryFailed, Message: "resource discovery failed"}
	ErrTokenNotFound      = &Response{Status: CodeTokenNotFound, Message: "token not found"}
	ErrRobotExists        = &Response{Status: CodeRobotExists, Message: "robot already exists"}
	ErrRobotNotFound      = &Response{Status: CodeRobotNotFound, Message: "robot not found"}
	ErrRobotExpired       = &Response{Status: CodeRobotExpired, Message: "robot token expired"}
	ErrPolicyNotFound     = &Response{Status: CodePolicyNotFound, Message: "policy not found"}
	ErrUserNotFound       = &Response{Status: CodeUserNotFound, Message: "user not found"}
	ErrUserExists         = &Response{Status: CodeUserExists, Message: "user already exists"}
	ErrGroupExists        = &Response{Status: CodeGroupExists, Message: "group already exists"}
	ErrGroupNotFound      = &Response{Status: CodeGroupNotFound, Message: "group not found"}
	ErrLDAPDisabled       = &Response{Status: CodeLDAPDisabled, Message: "ldap not configured"}
	ErrScriptsDisabled    = &Response{Status: CodeScriptsDisabled, Message: "mapping scripts disabled"}
	ErrAuthScriptNotFound = &Response{Status: CodeAuthScriptNotFound, Message: "script not found"}
)

// ServerInternal 系统内部错误：携带底层 err 详情，仅 handler 出口构造
func ServerInternal(err error) *Response {
	return &Response{Status: CodeServerInternal, Message: "internal error: " + err.Error()}
}

// IsSuccess 判定是否成功响应。nil = 成功（层间约定成功时返回 nil rsp，
// 只有 GenFinalResponse 出口把它归一为 OperationSuccess）。
func IsSuccess(rsp *Response) bool {
	return rsp == nil || errors.Is(rsp, OperationSuccess)
}

// GenFinalResponse 统一出口：HTTP 恒 200（系统错误 500），业务码在 status 字段。
// data 允许为 nil（错误响应不携带数据）。
func GenFinalResponse(w http.ResponseWriter, data any, rsp *Response) {
	if rsp == nil {
		rsp = OperationSuccess
	}
	code := http.StatusOK
	if !IsSuccess(rsp) && rsp.Status >= CodeServerInternal {
		code = http.StatusInternalServerError
	}

	body := struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Data    any    `json:"data,omitempty"`
	}{Status: rsp.Status, Message: rsp.Message, Data: data}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// 头部已写出，只能记录；连接将中断由调用方超时兜底
		fmt.Printf("[errcode] write response: %v\n", err)
	}
}
