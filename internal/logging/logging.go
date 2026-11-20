// Package logging 统一日志初始化：Zap JSON/Console 编码、级别与全局替换。
//
// 环境变量：
//   - AM_LOG_LEVEL   debug|info|warn|error（默认 info）
//   - AM_LOG_FORMAT  json|console（默认 json；console 便于本地阅读）
//
// 初始化后业务代码统一使用 zap.L()/zap.S()；未初始化时 Zap 全局为 no-op，
// 测试不会产生日志噪音。
package logging

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Init 初始化全局 Zap Logger 并返回用于 Sync 的清理函数（幂等）。
func Init(service string) (func(), error) {
	level, err := parseLevel(os.Getenv("AM_LOG_LEVEL"))
	if err != nil {
		return nil, err
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	if strings.EqualFold(strings.TrimSpace(os.Getenv("AM_LOG_FORMAT")), "console") {
		cfg.Encoding = "console"
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	}

	logger, err := cfg.Build(zap.Fields(zap.String("service", service)))
	if err != nil {
		return nil, fmt.Errorf("logging -> build: %w", err)
	}
	zap.ReplaceGlobals(logger)
	return func() { _ = logger.Sync() }, nil
}

// parseLevel 解析日志级别（空值默认 info）。
func parseLevel(raw string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return zapcore.InfoLevel, nil
	case "debug":
		return zapcore.DebugLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("logging -> unknown AM_LOG_LEVEL %q (want debug|info|warn|error)", raw)
	}
}
