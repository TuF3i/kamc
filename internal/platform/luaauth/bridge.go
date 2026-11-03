package luaauth

// 桥接注册：把平台 API 以原生 Go 函数暴露给 Lua（kamc.* 全局表）。
//
// 刻意不暴露的危险操作：Token 签发/吊销、集群删除、脚本管理——
// 桥接面向"用户数据映射与策略计算"，管理动作走管理面 API。
//
// dry-run 模式：所有变更动作只记录不执行（查询照常），返回动作清单。

import (
	lua "github.com/yuin/gopher-lua"

	"github.com/TuF3i/kamc/internal/platform/modules"
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
	boolRes := func(L *lua.LState, changed bool, err error, api string) int {
		if err != nil {
			L.RaiseError("%s: %v", api, err)
			return 0
		}
		L.Push(lua.LBool(changed))
		return 1
	}
	ctx := L.Context()

	// ── 用户/组（平台用户目录）──
	set("upsert_user", func(L *lua.LState) int {
		username, nickname, groups := str(L, 1, "upsert_user"), str(L, 2, "upsert_user"), str(L, 3, "upsert_user")
		if action("upsert_user", []string{username, nickname, groups}) {
			u := &modules.PlatformUser{Username: username, Nickname: nickname, Groups: groups, Source: modules.UserSource(claims.Source), Metadata: "{}"}
			if existing, err := e.users.ListUsersDirect(ctx); err == nil {
				for i := range existing {
					if existing[i].Username == username {
						u.UID = existing[i].UID // 保留既有 uid
						break
					}
				}
			}
			return boolRes(L, true, e.users.UpsertUserDirect(ctx, u), "upsert_user")
		}
		L.Push(lua.LTrue)
		return 1
	})
	set("list_users", func(L *lua.LState) int {
		users, err := e.users.ListUsersDirect(ctx)
		if err != nil {
			L.RaiseError("list_users: %v", err)
			return 0
		}
		t := L.NewTable()
		for i, u := range users {
			ut := L.NewTable()
			ut.RawSetString("uid", lua.LString(u.UID))
			ut.RawSetString("username", lua.LString(u.Username))
			ut.RawSetString("nickname", lua.LString(u.Nickname))
			ut.RawSetString("groups", lua.LString(u.Groups))
			t.RawSetInt(i+1, ut)
		}
		L.Push(t)
		return 1
	})
	set("upsert_group", func(L *lua.LState) int {
		name, desc := str(L, 1, "upsert_group"), str(L, 2, "upsert_group")
		if action("upsert_group", []string{name, desc}) {
			return boolRes(L, true, e.users.UpsertGroupDirect(ctx, name, desc, claims.Source), "upsert_group")
		}
		L.Push(lua.LTrue)
		return 1
	})
	set("list_groups", func(L *lua.LState) int {
		groups, err := e.users.ListGroupsDirect(ctx)
		if err != nil {
			L.RaiseError("list_groups: %v", err)
			return 0
		}
		t := L.NewTable()
		for i, g := range groups {
			gt := L.NewTable()
			gt.RawSetString("name", lua.LString(g.Name))
			gt.RawSetString("desc", lua.LString(g.Desc))
			t.RawSetInt(i+1, gt)
		}
		L.Push(t)
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
