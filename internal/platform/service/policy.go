package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// maxPolicyFieldLen 策略字段长度上限（防滥用；DB 列宽一致）
const maxPolicyFieldLen = 256

var allowedPolicyVerbs = map[string]bool{
	"get": true, "list": true, "watch": true,
	"create": true, "update": true, "patch": true, "delete": true,
	"deletecollection": true, "proxy": true, "use": true,
}

func validatePolicyFields(fields ...string) *errcode.Response {
	for _, f := range fields {
		if strings.TrimSpace(f) == "" || len(f) > maxPolicyFieldLen {
			return errcode.ErrInvalidParam
		}
	}
	return nil
}

// subjectValue 把 UI 的 org/user + 名称转换为 Casbin 主体前缀。
func subjectValue(subjectType, subject string) (string, *errcode.Response) {
	kind := strings.ToLower(strings.TrimSpace(subjectType))
	if kind != "org" && kind != "user" {
		return "", errcode.ErrInvalidParam
	}
	name := strings.TrimSpace(subject)
	if name == "" || len(name) > maxPolicyFieldLen {
		return "", errcode.ErrInvalidParam
	}
	if strings.HasPrefix(name, "org:") || strings.HasPrefix(name, "user:") {
		return "", errcode.ErrInvalidParam
	}
	return kind + ":" + name, nil
}

// normalizeVerbs 把多选动词转换为 Casbin act 正则片段；* 表示全部。
func normalizeVerbs(verbs []string) (string, *errcode.Response) {
	if len(verbs) == 0 {
		return "", errcode.ErrInvalidParam
	}
	seen := make(map[string]bool)
	parts := make([]string, 0, len(verbs))
	for _, raw := range verbs {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if v == "*" {
			return ".*", nil
		}
		if !allowedPolicyVerbs[v] {
			return "", errcode.ErrInvalidParam
		}
		if !seen[v] {
			seen[v] = true
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 {
		return "", errcode.ErrInvalidParam
	}
	return strings.Join(parts, "|"), nil
}

// expandRule 把一条 UI 资源规则展开为一条或多条 Casbin 四元组。
func expandRule(subject, cluster string, rule modules.PolicyRuleInput) ([][4]string, *errcode.Response) {
	resource := strings.TrimSpace(rule.Resource)
	if resource == "" {
		return nil, errcode.ErrInvalidParam
	}
	act, rsp := normalizeVerbs(rule.Verbs)
	if rsp != nil {
		return nil, rsp
	}
	if resource == "*" {
		return [][4]string{{subject, cluster, "*", act}}, nil
	}

	subresource := strings.TrimSpace(rule.Subresource)
	if rule.Namespaced {
		namespace := strings.TrimSpace(rule.Namespace)
		if namespace == "" {
			namespace = "*"
		}
		obj := namespace + "/" + resource
		if subresource != "" {
			obj += "/" + subresource
		}
		rows := [][4]string{{subject, cluster, obj, act}}
		// 全命名空间 list（/api/v1/pods）解析为 cluster/<resource>，需要额外覆盖。
		if namespace == "*" && subresource == "" {
			rows = append(rows, [4]string{subject, cluster, "cluster/" + resource, act})
		}
		return rows, nil
	}

	obj := "cluster/" + resource
	if subresource != "" {
		obj += "/" + subresource
	}
	rows := [][4]string{{subject, cluster, obj, act}}
	// namespaces 是 cluster-scoped 资源，但单个 namespace 的 get/update/delete
	// 路径解析为 <ns>/namespaces；额外覆盖这种路径。
	if resource == "namespaces" && subresource == "" {
		rows = append(rows, [4]string{subject, cluster, "*/namespaces", act})
	}
	return rows, nil
}

func parseObj(obj string) (namespace, resource, subresource string) {
	switch {
	case obj == "*":
		return "*", "*", ""
	case strings.HasPrefix(obj, "nonresource"):
		return "", obj, ""
	case strings.HasPrefix(obj, "cluster/"):
		rest := strings.TrimPrefix(obj, "cluster/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return "", parts[0], parts[1]
		}
		return "", rest, ""
	default:
		parts := strings.SplitN(obj, "/", 3)
		if len(parts) >= 2 {
			resource = parts[1]
			if len(parts) == 3 {
				subresource = parts[2]
			}
			return parts[0], resource, subresource
		}
		return "", obj, ""
	}
}

func parseAct(act string) []string {
	if act == ".*" || act == "*" {
		return []string{"*"}
	}
	parts := strings.Split(act, "|")
	verbs := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			verbs = append(verbs, p)
		}
	}
	return verbs
}

func policyDTOFromRow(row []string) modules.PolicyDTO {
	if len(row) < 4 {
		return modules.PolicyDTO{}
	}
	sub, dom, obj, act := row[0], row[1], row[2], row[3]
	subjectType, subject := "legacy", sub
	if strings.HasPrefix(sub, "user:") {
		subjectType, subject = "user", strings.TrimPrefix(sub, "user:")
	} else if strings.HasPrefix(sub, "org:") {
		subjectType, subject = "org", strings.TrimPrefix(sub, "org:")
	}
	namespace, resource, subresource := parseObj(obj)
	return modules.PolicyDTO{
		SubjectType: subjectType,
		Subject:     subject,
		Cluster:     dom,
		Namespace:   namespace,
		Resource:    resource,
		Subresource: subresource,
		Verbs:       parseAct(act),
		Sub:         sub,
		Dom:         dom,
		Obj:         obj,
		Act:         act,
	}
}

// CreatePolicies 批量创建策略：一条 UI 规则可能展开为多条 Casbin 行。
func (s *Service) CreatePolicies(ctx context.Context, req *modules.CreatePolicyRequest) (*modules.CreatePolicyResult, *errcode.Response) {
	if req == nil {
		return nil, errcode.ErrInvalidParam
	}
	subject, rsp := subjectValue(req.SubjectType, req.Subject)
	if rsp != nil {
		return nil, rsp
	}
	cluster := strings.TrimSpace(req.Cluster)
	if cluster == "" || len(req.Rules) == 0 {
		return nil, errcode.ErrInvalidParam
	}
	if _, rsp := s.getCluster(ctx, cluster); rsp != nil {
		return nil, rsp
	}

	rows := make([][4]string, 0, len(req.Rules))
	seen := make(map[[4]string]bool)
	for _, rule := range req.Rules {
		expanded, rsp := expandRule(subject, cluster, rule)
		if rsp != nil {
			return nil, rsp
		}
		for _, row := range expanded {
			if !seen[row] {
				seen[row] = true
				rows = append(rows, row)
			}
		}
	}
	if len(rows) == 0 {
		return nil, errcode.ErrInvalidParam
	}

	added := make([][4]string, 0, len(rows))
	policies := make([]modules.PolicyDTO, 0, len(rows))
	for _, row := range rows {
		ok, err := s.authz.AddPolicy(row[0], row[1], row[2], row[3])
		if err != nil {
			for _, done := range added {
				_, _ = s.authz.RemovePolicy(done[0], done[1], done[2], done[3])
			}
			return nil, errcode.ServerInternal(err)
		}
		if ok {
			added = append(added, row)
			policies = append(policies, policyDTOFromRow(row[:]))
		}
	}
	return &modules.CreatePolicyResult{Created: len(added), Policies: policies}, nil
}

// ListPolicies 全量策略并解析为 UI DTO。
func (s *Service) ListPolicies() ([]modules.PolicyDTO, *errcode.Response) {
	rules, err := s.authz.ListPolicies()
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	dtos := make([]modules.PolicyDTO, 0, len(rules))
	for _, row := range rules {
		if len(row) == 4 {
			dtos = append(dtos, policyDTOFromRow(row))
		}
	}
	return dtos, nil
}

// RemovePolicy 删除精确策略四元组。
func (s *Service) RemovePolicy(_ context.Context, req modules.RemovePolicyRequest) *errcode.Response {
	if rsp := validatePolicyFields(req.Sub, req.Dom, req.Obj, req.Act); rsp != nil {
		return rsp
	}
	removed, err := s.authz.RemovePolicy(req.Sub, req.Dom, req.Obj, req.Act)
	if err != nil {
		return errcode.ServerInternal(err)
	}
	if !removed {
		return errcode.ErrPolicyNotFound
	}
	return nil
}

// RemovePoliciesBySubject 删除某个主体的全部策略（Robot 吊销时使用）。
func (s *Service) RemovePoliciesBySubject(_ context.Context, sub string) (int, *errcode.Response) {
	if strings.TrimSpace(sub) == "" {
		return 0, errcode.ErrInvalidParam
	}
	rules, err := s.authz.ListPolicies()
	if err != nil {
		return 0, errcode.ServerInternal(err)
	}
	removed := 0
	for _, row := range rules {
		if len(row) != 4 || row[0] != sub {
			continue
		}
		ok, err := s.authz.RemovePolicy(row[0], row[1], row[2], row[3])
		if err != nil {
			return removed, errcode.ServerInternal(fmt.Errorf("remove policies by subject: %w", err))
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

// Enforce 数据面鉴权判定（/proxy 专用；fail-closed）。
func (s *Service) Enforce(sub string, groups []string, dom, obj, act string) (bool, error) {
	return s.authz.Enforce(sub, groups, dom, obj, act)
}
