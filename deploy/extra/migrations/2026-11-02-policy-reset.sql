-- 权限模型重构（组织/用户直接策略）：
-- 旧角色/用户绑定策略不再兼容，部署新后端前执行一次。
DELETE FROM policy_rules;
DROP TABLE IF EXISTS roles;
ALTER TABLE robots DROP COLUMN IF EXISTS role;
-- 绑定脚本功能已移除（历史表不再使用）
DROP TABLE IF EXISTS auth_scripts;
