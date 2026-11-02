package luaauth

// 桥接注册：把 kamc 权限 API 以原生 Go 函数暴露给 Lua（kamc.* 全局表）。
//
// 刻意不暴露的危险操作：Token 签发/吊销、用户踢出、集群删除——
// 桥接面向"绑定/策略计算"，管理动作走管理面 API。
//
// dry-run 模式：所有变更动作只记录不执行（查询照常），返回动作清单。

import (
	lua "github.com/yuin/gopher-lua"
)

func (e *Engine) registerBridge(L *lua.LState, claims Claims, dryRun bool, record func(api string, args []string)) {
	kamc := L.NewTable()

	type luaFn = func(L *lua.LState) int
	set := func(name string, fn luaFn) { kamc.RawSetString(name, L.NewClosure(fn)) }

	// 参数读取助手：越界/类型错直接 RaiseError（转为执行错误，fail-closed）
	str := func(L *lua.LState, n int, api string) string {
		v := L.Get(n)
		if v.Type() != lua.LTString {
			L.RaiseError("%s: arg#%d must be string", api, n)
			return ""
		}
		return string(v.(lua.LString))
	}
	action := func(api string, args []string) bool {
		record(api, args)
		return !dryRun
	}
	boolRes := func(L *lua.LState, added bool, err error, api string) int {
		if err != nil {
			L.RaiseError("%s: %v", api, err)
			return 0
		}
		L.Push(lua.LBool(added))
		return 1
	}

	// ── 集群读取 ──
	set("list_clusters", func(L *lua.LState) int {
		clusters, err := e.clusters.ListClusters(L.Context())
		if err != nil {
			L.RaiseError("list_clusters: %v", err)
			return 0
		}
		t := L.NewTable()
		for i, c := range clusters {
			ct := L.NewTable()
			ct.RawSetString("id", lua.LString(c.ID))
			ct.RawSetString("name", lua.LString(c.Name))
			ct.RawSetString("status", lua.LString(c.Status))
			t.RawSetInt(i+1, ct)
		}
		L.Push(t)
		return 1
	})
	set("cluster_exists", func(L *lua.LState) int {
		dom := str(L, 1, "cluster_exists")
		_, err := e.clusters.GetCluster(L.Context(), dom)
		if err != nil {
			L.Push(lua.LFalse)
			return 1
		}
		L.Push(lua.LTrue)
		return 1
	})

	// ── 绑定 ──
	set("list_bindings", func(L *lua.LState) int {
		rows, err := e.authz.ListBindings()
		if err != nil {
			L.RaiseError("list_bindings: %v", err)
			return 0
		}
		return pushRows(L, rows, 3)
	})
	set("add_binding", func(L *lua.LState) int {
		user, role, dom := str(L, 1, "add_binding"), str(L, 2, "add_binding"), str(L, 3, "add_binding")
		if action("add_binding", []string{user, role, dom}) {
			added, err := e.authz.AddBinding(user, role, dom)
			return boolRes(L, added, err, "add_binding")
		}
		L.Push(lua.LTrue)
		return 1
	})
	set("remove_binding", func(L *lua.LState) int {
		user, role, dom := str(L, 1, "remove_binding"), str(L, 2, "remove_binding"), str(L, 3, "remove_binding")
		if action("remove_binding", []string{user, role, dom}) {
			removed, err := e.authz.RemoveBinding(user, role, dom)
			return boolRes(L, removed, err, "remove_binding")
		}
		L.Push(lua.LTrue)
		return 1
	})

	// ── 策略 ──
	set("list_policies", func(L *lua.LState) int {
		rows, err := e.authz.ListPolicies()
		if err != nil {
			L.RaiseError("list_policies: %v", err)
			return 0
		}
		return pushRows(L, rows, 4)
	})
	set("has_policy", func(L *lua.LState) int {
		sub, dom, obj, act := str(L, 1, "has_policy"), str(L, 2, "has_policy"), str(L, 3, "has_policy"), str(L, 4, "has_policy")
		ok, err := e.authz.HasPolicy(sub, dom, obj, act)
		if err != nil {
			L.RaiseError("has_policy: %v", err)
			return 0
		}
		L.Push(lua.LBool(ok))
		return 1
	})
	set("add_policy", func(L *lua.LState) int {
		sub, dom, obj, act := str(L, 1, "add_policy"), str(L, 2, "add_policy"), str(L, 3, "add_policy"), str(L, 4, "add_policy")
		if action("add_policy", []string{sub, dom, obj, act}) {
			added, err := e.authz.AddPolicy(sub, dom, obj, act)
			return boolRes(L, added, err, "add_policy")
		}
		L.Push(lua.LTrue)
		return 1
	})
	set("remove_policy", func(L *lua.LState) int {
		sub, dom, obj, act := str(L, 1, "remove_policy"), str(L, 2, "remove_policy"), str(L, 3, "remove_policy"), str(L, 4, "remove_policy")
		if action("remove_policy", []string{sub, dom, obj, act}) {
			removed, err := e.authz.RemovePolicy(sub, dom, obj, act)
			return boolRes(L, removed, err, "remove_policy")
		}
		L.Push(lua.LTrue)
		return 1
	})

	L.SetGlobal("kamc", kamc)
}

// pushRows 把 [n]string 行集转为 Lua 表（索引从 1 起）
func pushRows(L *lua.LState, rows [][]string, width int) int {
	t := L.NewTable()
	for i, row := range rows {
		rt := L.NewTable()
		for j := 0; j < width && j < len(row); j++ {
			rt.RawSetInt(j+1, lua.LString(row[j]))
		}
		t.RawSetInt(i+1, rt)
	}
	L.Push(t)
	return 1
}

// bridgeNames 供文档与测试断言的桥接清单
func bridgeNames() []string {
	return []string{
		"list_clusters", "cluster_exists",
		"list_bindings", "add_binding", "remove_binding",
		"has_policy", "add_policy", "remove_policy", "list_policies",
	}
}
