package service

import (
	"testing"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

func TestSubjectValue(t *testing.T) {
	if got, rsp := subjectValue("org", "dev-team"); rsp != nil || got != "org:dev-team" {
		t.Fatalf("org: got=%q rsp=%v", got, rsp)
	}
	if got, rsp := subjectValue("user", "uid-alice"); rsp != nil || got != "user:uid-alice" {
		t.Fatalf("user: got=%q rsp=%v", got, rsp)
	}
	if _, rsp := subjectValue("role", "x"); rsp == nil {
		t.Fatal("role subject type should be rejected")
	}
	if _, rsp := subjectValue("user", "user:already"); rsp == nil {
		t.Fatal("prefixed subject should be rejected")
	}
}

func TestExpandRule(t *testing.T) {
	cases := []struct {
		name string
		rule modules.PolicyRuleInput
		want [][4]string
	}{
		{
			name: "namespaced all namespaces",
			rule: modules.PolicyRuleInput{Namespace: "*", Resource: "pods", Namespaced: true, Verbs: []string{"get", "list"}},
			want: [][4]string{
				{"user:u", "c", "*/pods", "get|list"},
				{"user:u", "c", "cluster/pods", "get|list"},
			},
		},
		{
			name: "namespaced specific namespace subresource",
			rule: modules.PolicyRuleInput{Namespace: "dev", Resource: "pods", Subresource: "exec", Namespaced: true, Verbs: []string{"create"}},
			want: [][4]string{{"user:u", "c", "dev/pods/exec", "create"}},
		},
		{
			name: "cluster scoped",
			rule: modules.PolicyRuleInput{Resource: "nodes", Namespaced: false, Verbs: []string{"get", "list"}},
			want: [][4]string{{"user:u", "c", "cluster/nodes", "get|list"}},
		},
		{
			name: "namespaces special case",
			rule: modules.PolicyRuleInput{Resource: "namespaces", Namespaced: false, Verbs: []string{"get"}},
			want: [][4]string{
				{"user:u", "c", "cluster/namespaces", "get"},
				{"user:u", "c", "*/namespaces", "get"},
			},
		},
		{
			name: "wildcard resource",
			rule: modules.PolicyRuleInput{Resource: "*", Namespaced: false, Verbs: []string{"*"}},
			want: [][4]string{{"user:u", "c", "*", ".*"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, rsp := expandRule("user:u", "c", tc.rule)
			if rsp != nil {
				t.Fatalf("expand: %v", rsp)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("rows: got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("row %d: got %v want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestPolicyDTOFromRow(t *testing.T) {
	dto := policyDTOFromRow([]string{"org:dev-team", "prod-bj", "*/pods/exec", "create|get"})
	if dto.SubjectType != "org" || dto.Subject != "dev-team" || dto.Cluster != "prod-bj" {
		t.Fatalf("subject: %+v", dto)
	}
	if dto.Namespace != "*" || dto.Resource != "pods" || dto.Subresource != "exec" {
		t.Fatalf("obj: %+v", dto)
	}
	if len(dto.Verbs) != 2 || dto.Verbs[0] != "create" || dto.Verbs[1] != "get" {
		t.Fatalf("verbs: %+v", dto.Verbs)
	}

	wild := policyDTOFromRow([]string{"user:u", "c", "*", ".*"})
	if wild.Namespace != "*" || wild.Resource != "*" || len(wild.Verbs) != 1 || wild.Verbs[0] != "*" {
		t.Fatalf("wildcard: %+v", wild)
	}
}
