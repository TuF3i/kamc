// Package version 集中管理构建版本信息，经 -ldflags 注入（见 Makefile）。
package version

// Version 语义化版本号，发布打 tag 时由 CI 覆盖
var Version = "dev"

// Commit 构建时注入的 git commit 短哈希
var Commit = "none"
