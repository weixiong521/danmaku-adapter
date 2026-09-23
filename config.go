package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config 运行配置。
//
// 加载优先级：环境变量 > config.json > 内置默认值。
// 支持运行时热更新：修改 config.json 后由后台 Watcher 自动热重载；
// 也可通过 /admin 接口即时增删“资源站”，无需重启进程。
type Config struct {
	Listen         string        // 监听地址，默认 :12381
	LogVarBase     string        // LogVar 部署根地址（不含 /api）
	LogVarToken    string        // 可选：LogVar 访问令牌
	ResourceHosts  []string      // 资源站主机池（分享页 + CDN），支持热更新
	HTTPTimeout    time.Duration // 单次上游请求超时
	MaxDanmu       int           // 单集弹幕上限，0 表示不限制
	FontSize       string        // Getapp 弹幕字号字段
	DefaultColor   string        // 颜色解析失败时的兜底颜色
	SourcePriority []string      // 选源偏好顺序（命中越靠前加分越高）
	ResolveTTL     time.Duration // “剧名/季/集 -> episodeId” 解析缓存时间

	AdminEnabled  bool          // 是否开启 /admin 管理接口
	AdminToken    string        // /admin 访问令牌（请求头 X-Admin-Token）
	WatchInterval time.Duration // 配置文件轮询间隔
	ConfigPath    string        // config.json 路径
}

// fileConfig 与 config.json 一一对应。
type fileConfig struct {
	Listen         string   `json:"listen"`
	LogVarBase     string   `json:"logvar_base"`
	LogVarToken    string   `json:"logvar_token"`
	ResourceHosts  []string `json:"resource_hosts"`
	ShareHosts     []string `json:"share_hosts"` // 兼容旧字段
	HTTPTimeoutMs  int      `json:"http_timeout_ms"`
	MaxDanmu       int      `json:"max_danmu"`
	FontSize       string   `json:"font_size"`
	DefaultColor   string   `json:"default_color"`
	SourcePriority []string `json:"source_priority"`
	ResolveTTLSec  int      `json:"resolve_ttl_sec"`

	AdminEnabled     *bool  `json:"admin_enabled"`
	AdminToken       string `json:"admin_token"`
	WatchIntervalSec int    `json:"watch_interval_sec"`
}

func env(key string) string { return strings.TrimSpace(os.Getenv(key)) }

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "y":
		return true
	}
	return false
}

func defaultConfig() *Config {
	return &Config{
		Listen:         ":12381",
		LogVarBase:     "https://1.501710491.xyz/weixiong",
		ResourceHosts:  []string{"https://jimaoys95.com", "https://jimaoys94.com"},
		HTTPTimeout:    45 * time.Second,
		MaxDanmu:       0,
		FontSize:       "24px",
		DefaultColor:   "#FFFFFF",
		SourcePriority: []string{"tencent", "iqiyi", "bilibili", "youku", "mango", "migu", "renren", "kan360", "douban", "vod"},
		ResolveTTL:     10 * time.Minute,
		AdminEnabled:   false,
		AdminToken:     "",
		WatchInterval:  15 * time.Second,
	}
}

// clone 深拷贝，保证热更新时读方拿到的是稳定快照。
func (c *Config) clone() *Config {
	cp := *c
	cp.ResourceHosts = append([]string(nil), c.ResourceHosts...)
	cp.SourcePriority = append([]string(nil), c.SourcePriority...)
	return &cp
}

// loadConfigFrom 读取 config.json 并叠加环境变量，生成一份完整配置。
func loadConfigFrom(path string) *Config {
	cfg := defaultConfig()
	cfg.ConfigPath = path

	var fc fileConfig
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, &fc); err != nil {
				log.Printf("[config] 解析 %s 失败：%v（继续使用默认值 + 环境变量）", path, err)
			} else {
				if fc.Listen != "" {
					cfg.Listen = fc.Listen
				}
				if fc.LogVarBase != "" {
					cfg.LogVarBase = fc.LogVarBase
				}
				if fc.LogVarToken != "" {
					cfg.LogVarToken = fc.LogVarToken
				}
				hosts := fc.ResourceHosts
				if len(hosts) == 0 {
					hosts = fc.ShareHosts // 兼容旧字段 share_hosts
				}
				if len(hosts) > 0 {
					cfg.ResourceHosts = hosts
				}
				if fc.HTTPTimeoutMs > 0 {
					cfg.HTTPTimeout = time.Duration(fc.HTTPTimeoutMs) * time.Millisecond
				}
				if fc.MaxDanmu > 0 {
					cfg.MaxDanmu = fc.MaxDanmu
				}
				if fc.FontSize != "" {
					cfg.FontSize = fc.FontSize
				}
				if fc.DefaultColor != "" {
					cfg.DefaultColor = fc.DefaultColor
				}
				if len(fc.SourcePriority) > 0 {
					cfg.SourcePriority = fc.SourcePriority
				}
				if fc.ResolveTTLSec > 0 {
					cfg.ResolveTTL = time.Duration(fc.ResolveTTLSec) * time.Second
				}
				if fc.AdminEnabled != nil {
					cfg.AdminEnabled = *fc.AdminEnabled
				}
				if fc.AdminToken != "" {
					cfg.AdminToken = fc.AdminToken
				}
				if fc.WatchIntervalSec > 0 {
					cfg.WatchInterval = time.Duration(fc.WatchIntervalSec) * time.Second
				}
			}
		}
	}

	// ---- 环境变量覆盖（优先级最高）----
	if v := env("LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := env("PORT"); v != "" {
		if !strings.Contains(v, ":") {
			v = ":" + v
		}
		cfg.Listen = v
	}
	if v := env("LOGVAR_BASE"); v != "" {
		cfg.LogVarBase = v
	}
	if v := env("LOGVAR_TOKEN"); v != "" {
		cfg.LogVarToken = v
	}
	if v := env("RESOURCE_HOSTS"); v != "" {
		cfg.ResourceHosts = splitCSV(v)
	} else if v := env("JULIANG_SHARE_HOSTS"); v != "" {
		cfg.ResourceHosts = splitCSV(v)
	}
	if v := env("HTTP_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.HTTPTimeout = time.Duration(n) * time.Millisecond
		}
	}
	if v := env("MAX_DANMU"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.MaxDanmu = n
		}
	}
	if v := env("FONT_SIZE"); v != "" {
		cfg.FontSize = v
	}
	if v := env("DEFAULT_COLOR"); v != "" {
		cfg.DefaultColor = v
	}
	if v := env("SOURCE_PRIORITY"); v != "" {
		cfg.SourcePriority = splitCSV(v)
	}
	if v := env("RESOLVE_TTL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ResolveTTL = time.Duration(n) * time.Second
		}
	}
	if v := env("ADMIN_ENABLED"); v != "" {
		cfg.AdminEnabled = parseBool(v)
	}
	if v := env("ADMIN_TOKEN"); v != "" {
		cfg.AdminToken = v
	}
	if v := env("WATCH_INTERVAL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.WatchInterval = time.Duration(n) * time.Second
		}
	}
	if v := env("CONFIG_PATH"); v != "" {
		cfg.ConfigPath = v
	}

	cfg.LogVarBase = strings.TrimRight(cfg.LogVarBase, "/")
	cfg.ResourceHosts = normalizeHosts(cfg.ResourceHosts)
	cfg.SourcePriority = splitCSV(strings.Join(cfg.SourcePriority, ","))
	return cfg
}

// ConfigStore 持有“当前生效配置”，支持原子替换与热重载。
type ConfigStore struct {
	mu  sync.RWMutex
	cur *Config

	path string

	modMu   sync.Mutex
	lastMod time.Time
}

func NewConfigStore(path string) *ConfigStore {
	cur := loadConfigFrom(path)
	s := &ConfigStore{cur: cur, path: cur.ConfigPath}
	s.lastMod = fileModTime(cur.ConfigPath)
	log.Printf("[config] 已加载配置：路径=%q 资源站=%d 个 管理接口=%v",
		displayPath(cur.ConfigPath), len(cur.ResourceHosts), cur.AdminEnabled)
	return s
}

// Get 获取当前配置快照（只读，请勿原地修改）。
func (s *ConfigStore) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Set 原子替换当前配置。
func (s *ConfigStore) Set(c *Config) {
	s.mu.Lock()
	s.cur = c
	s.mu.Unlock()
}

func (s *ConfigStore) Path() string { return s.path }

// Reload 依据磁盘文件 + 环境变量重建配置（热重载核心）。
func (s *ConfigStore) Reload() *Config {
	c := loadConfigFrom(s.path)
	s.Set(c)
	s.touchMod()
	return c
}

func (s *ConfigStore) touchMod() {
	s.modMu.Lock()
	s.lastMod = fileModTime(s.path)
	s.modMu.Unlock()
}

// Persist 把当前配置写回 config.json（供 /admin 接口使用）。
func (s *ConfigStore) Persist() error {
	if s.path == "" {
		return nil
	}
	cfg := s.Get()
	data, err := json.MarshalIndent(toFileConfig(cfg), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return err
	}
	s.touchMod()
	return nil
}

// Watch 后台轮询配置文件 mtime，发生变化即热重载（默认每 15s）。
func (s *ConfigStore) Watch(ctx context.Context) {
	interval := s.Get().WatchInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	log.Printf("[config] 配置热重载已启动：监听 %q，间隔 %s", displayPath(s.path), interval)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if s.changed() {
				log.Printf("[config] 检测到 %q 变更，执行热重载", displayPath(s.path))
				s.Reload()
			}
		}
	}
}

func (s *ConfigStore) changed() bool {
	s.modMu.Lock()
	defer s.modMu.Unlock()
	mod := fileModTime(s.path)
	if mod.IsZero() || mod.Equal(s.lastMod) {
		return false
	}
	s.lastMod = mod
	return true
}

func fileModTime(path string) time.Time {
	if path == "" {
		return time.Time{}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

func toFileConfig(c *Config) fileConfig {
	admin := c.AdminEnabled
	return fileConfig{
		Listen:           c.Listen,
		LogVarBase:       c.LogVarBase,
		LogVarToken:      c.LogVarToken,
		ResourceHosts:    c.ResourceHosts,
		HTTPTimeoutMs:    int(c.HTTPTimeout / time.Millisecond),
		MaxDanmu:         c.MaxDanmu,
		FontSize:         c.FontSize,
		DefaultColor:     c.DefaultColor,
		SourcePriority:   c.SourcePriority,
		ResolveTTLSec:    int(c.ResolveTTL / time.Second),
		AdminEnabled:     &admin,
		AdminToken:       c.AdminToken,
		WatchIntervalSec: int(c.WatchInterval / time.Second),
	}
}

func displayPath(p string) string {
	if p == "" {
		return "(默认值)"
	}
	return p
}
