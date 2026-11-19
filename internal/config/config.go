// Package config 统一的配置装载层（Viper）：默认值 → 配置文件 → 环境变量。
//
// 约定：
//   - 环境变量名保持历史契约（AM_* / DATABASE_URL / OIDC_* …），部署清单无需改动；
//   - 配置文件通过 AM_CONFIG_FILE 或 -config 指定，未指定时在当前目录与 /etc/kamc
//     搜索 <name>.yaml/<name>.yml/<name>.json/<name>.toml；
//   - 环境变量优先于配置文件（12-factor），显式注入的 Config（测试）优先于两者。
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// EnvConfigFile 指定配置文件的元环境变量（不走 Viper 键绑定）。
const EnvConfigFile = "AM_CONFIG_FILE"

// Loader 配置装载器（每个进程/组件独立实例，测试之间不共享状态）。
type Loader struct {
	v *viper.Viper
}

// New 创建装载器。
func New() *Loader {
	return &Loader{v: viper.New()}
}

// SetDefault 设置默认值（最低优先级）。
func (l *Loader) SetDefault(key string, value any) {
	l.v.SetDefault(key, value)
}

// Bind 绑定环境变量（支持多个候选名，按顺序取第一个非空值）。
func (l *Loader) Bind(key string, envNames ...string) {
	if len(envNames) == 0 {
		return
	}
	_ = l.v.BindEnv(append([]string{key}, envNames...)...)
}

// LoadFile 读取配置文件：
//   - path 非空：必须存在且可解析；
//   - path 为空：读 AM_CONFIG_FILE；仍未设置则在 "." 与 "/etc/kamc" 搜索 baseName；
//   - baseName 为空时使用 "kamc"。
func (l *Loader) LoadFile(path, baseName string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = strings.TrimSpace(os.Getenv(EnvConfigFile))
	}
	if path != "" {
		l.v.SetConfigFile(path)
		if err := l.v.ReadInConfig(); err != nil {
			return fmt.Errorf("config -> read %s: %w", path, err)
		}
		return nil
	}
	if baseName == "" {
		baseName = "kamc"
	}
	l.v.SetConfigName(baseName)
	l.v.AddConfigPath(".")
	l.v.AddConfigPath("/etc/kamc")
	if err := l.v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return nil // 无配置文件：仅默认值 + 环境变量
		}
		return fmt.Errorf("config -> read %s: %w", baseName, err)
	}
	return nil
}

// FileUsed 返回实际加载的配置文件（未加载返回空）。
func (l *Loader) FileUsed() string {
	return l.v.ConfigFileUsed()
}

// String 读取字符串（不去空白，密码可能含空格）。
func (l *Loader) String(key string) string {
	return l.v.GetString(key)
}

// Bool 读取布尔值；未配置时返回 def。
func (l *Loader) Bool(key string, def bool) bool {
	if !l.v.IsSet(key) {
		return def
	}
	return l.v.GetBool(key)
}

// Int 读取整数；未配置/解析失败时返回 def。
func (l *Loader) Int(key string, def int) int {
	if !l.v.IsSet(key) {
		return def
	}
	return l.v.GetInt(key)
}

// Duration 读取时间间隔（如 "30s"/"720h"/"0"）；未配置或解析失败返回 def。
func (l *Loader) Duration(key string, def time.Duration) time.Duration {
	if !l.v.IsSet(key) {
		return def
	}
	raw := strings.TrimSpace(l.v.GetString(key))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return def
	}
	return d
}

// StringList 读取字符串列表：配置文件可为数组，环境变量为逗号分隔。
func (l *Loader) StringList(key string) []string {
	if !l.v.IsSet(key) {
		return nil
	}
	if raw := l.v.GetString(key); raw != "" {
		var out []string
		for _, part := range strings.Split(raw, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if values := l.v.GetStringSlice(key); len(values) > 0 {
		return values
	}
	return nil
}

// IsSet 键是否在任意层（默认值/文件/环境）被设置。
func (l *Loader) IsSet(key string) bool {
	return l.v.IsSet(key)
}
