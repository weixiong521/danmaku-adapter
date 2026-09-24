package main

import (
	"net/url"
	"strings"
)

// SourceManager 负责“资源站”运行时的增删改查。
//
// 设计要点：
//   - 每次修改都基于当前配置 clone 一份，改完整体原子替换（ConfigStore.Set），
//     并立即 Persist 回 config.json，做到“实时替换/添加 + 重启不丢”。
//   - 并发请求读到的是稳定快照，不存在边改边读的竞态。
type SourceManager struct {
	store *ConfigStore
}

func NewSourceManager(store *ConfigStore) *SourceManager {
	return &SourceManager{store: store}
}

// Snapshot 返回资源站相关配置，供 GET /admin/sources 查询。
func (m *SourceManager) Snapshot() map[string]any {
	cfg := m.store.Get()
	return map[string]any{
		"resource_hosts":  cfg.ResourceHosts,
		"source_priority": cfg.SourcePriority,
		"logvar_base":     cfg.LogVarBase,
		"logvar_token":    maskToken(cfg.LogVarToken),
		"count":           len(cfg.ResourceHosts),
	}
}

// normalizeHost 规范化资源站主机：无协议补 https://、去尾斜杠、统一小写。
//
//	"jimaoys95.com"          -> "https://jimaoys95.com"
//	"https://Jimaoys95.com/" -> "https://jimaoys95.com"
func normalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	if !strings.Contains(h, "://") {
		h = "https://" + h
	}
	h = strings.TrimRight(h, "/")
	if u, err := url.Parse(h); err == nil && u.Host != "" {
		return strings.ToLower(u.Scheme + "://" + u.Host)
	}
	return strings.ToLower(h)
}

// normalizeHosts 规范化并去重，保持输入顺序。
func normalizeHosts(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, h := range in {
		n := normalizeHost(h)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// AddResourceHosts 批量新增资源站；返回实际新增项与当前全量列表。
func (m *SourceManager) AddResourceHosts(hosts []string) (added []string, current []string) {
	cfg := m.store.Get().clone()
	seen := make(map[string]bool, len(cfg.ResourceHosts))
	for _, h := range cfg.ResourceHosts {
		seen[h] = true
	}
	for _, h := range normalizeHosts(hosts) {
		if seen[h] {
			continue
		}
		seen[h] = true
		cfg.ResourceHosts = append(cfg.ResourceHosts, h)
		added = append(added, h)
	}
	m.store.Set(cfg)
	_ = m.store.Persist()
	return added, cfg.ResourceHosts
}

// RemoveResourceHosts 批量删除资源站；返回实际删除项与当前全量列表。
func (m *SourceManager) RemoveResourceHosts(hosts []string) (removed []string, current []string) {
	drop := make(map[string]bool, len(hosts))
	for _, h := range normalizeHosts(hosts) {
		drop[h] = true
	}
	cfg := m.store.Get().clone()
	kept := make([]string, 0, len(cfg.ResourceHosts))
	for _, h := range cfg.ResourceHosts {
		if drop[h] {
			removed = append(removed, h)
			continue
		}
		kept = append(kept, h)
	}
	cfg.ResourceHosts = kept
	m.store.Set(cfg)
	_ = m.store.Persist()
	return removed, cfg.ResourceHosts
}

// SetResourceHosts 整体替换资源站列表（清空后写入）。
func (m *SourceManager) SetResourceHosts(hosts []string) []string {
	cfg := m.store.Get().clone()
	cfg.ResourceHosts = normalizeHosts(hosts)
	m.store.Set(cfg)
	_ = m.store.Persist()
	return cfg.ResourceHosts
}

// SetSourcePriority 替换选源优先级（如 tencent,iqiyi,bilibili...）。
func (m *SourceManager) SetSourcePriority(list []string) []string {
	cfg := m.store.Get().clone()
	cfg.SourcePriority = splitCSV(strings.Join(list, ","))
	m.store.Set(cfg)
	_ = m.store.Persist()
	return cfg.SourcePriority
}

// SetLogVar 替换 LogVar 上游地址与令牌。
func (m *SourceManager) SetLogVar(base, token string) {
	cfg := m.store.Get().clone()
	if strings.TrimSpace(base) != "" {
		cfg.LogVarBase = strings.TrimRight(strings.TrimSpace(base), "/")
	}
	if strings.TrimSpace(token) != "" {
		cfg.LogVarToken = strings.TrimSpace(token)
	}
	m.store.Set(cfg)
	_ = m.store.Persist()
}

// HasResourceHost 判断某个播放地址是否命中已知资源站（用于识别播放源）。
func (m *SourceManager) HasResourceHost(rawURL string) bool {
	low := strings.ToLower(rawURL)
	for _, h := range m.store.Get().ResourceHosts {
		host := strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
		if host != "" && strings.Contains(low, host) {
			return true
		}
	}
	return false
}

func maskToken(t string) string {
	if t == "" {
		return ""
	}
	if len(t) <= 4 {
		return "****"
	}
	return t[:2] + strings.Repeat("*", len(t)-4) + t[len(t)-2:]
}
