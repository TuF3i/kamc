package postgres

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// gormLogger 将 GORM 日志接入 Zap（慢查询/错误），忽略记录不存在。
type gormLogger struct {
	level         logger.LogLevel
	slowThreshold time.Duration
}

// newGormLogger GORM 的 Zap 适配器：默认只记录慢查询与真实错误。
func newGormLogger() logger.Interface {
	return &gormLogger{level: logger.Warn, slowThreshold: 200 * time.Millisecond}
}

func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	clone := *l
	clone.level = level
	return &clone
}

func (l *gormLogger) Info(_ context.Context, msg string, args ...interface{}) {
	if l.level >= logger.Info {
		zap.S().Infof(msg, args...)
	}
}

func (l *gormLogger) Warn(_ context.Context, msg string, args ...interface{}) {
	if l.level >= logger.Warn {
		zap.S().Warnf(msg, args...)
	}
}

func (l *gormLogger) Error(_ context.Context, msg string, args ...interface{}) {
	if l.level >= logger.Error {
		zap.S().Errorf(msg, args...)
	}
}

func (l *gormLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		sql, rows := fc()
		zap.L().Warn("gorm query error",
			zap.Error(err),
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
		)
		return
	}
	if elapsed > l.slowThreshold {
		sql, rows := fc()
		zap.L().Warn("gorm slow query",
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
		)
		return
	}
	if l.level >= logger.Info {
		sql, rows := fc()
		zap.L().Debug("gorm query",
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
		)
	}
}
