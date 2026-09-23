package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	reJLHash    = regexp.MustCompile(`[0-9a-f]{32}`)
	reShareBoot = regexp.MustCompile(`(?s)<script id="share-boot"[^>]*>(.*?)</script>`)
)

// ---- 巨量分享页 #share-boot 结构 ----

type NavUnit struct {
	UnitID        string `json:"unitId"`
	Label         string `json:"label"`
	UnitKind      string `json:"unitKind"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
}

type ShareBoot struct {
	ShareKey     string    `json:"shareKey"`
	Title        string    `json:"title"`
	ContentID    string    `json:"contentId"`
	ContentTitle string    `json:"contentTitle"`
	Unit         string    `json:"unit"`
	Navigation   []NavUnit `json:"navigation"`
}

type JuliangResolver struct {
	store  *ConfigStore
	client *http.Client
	cache  *ttlCache
}

func NewJuliangResolver(store *ConfigStore) *JuliangResolver {
	return &JuliangResolver{store: store, client: &http.Client{}, cache: newTTLCache()}
}

// ExtractHash 从播放地址中提取 32 位内容标识。
// looksJL 为 true 表示判断该地址属于“可解析资源站”：
//   - 命中已配置的资源站主机（resource_hosts）
//   - 或包含巨量特征（jimaoys / /playback/ / /s/）
func ExtractHash(rawURL string, resourceHosts []string) (string, bool) {
	if rawURL == "" {
		return "", false
	}
	low := strings.ToLower(rawURL)
	looksJL := strings.Contains(low, "jimaoys") ||
		strings.Contains(low, "/playback/") ||
		strings.Contains(low, "/s/")
	if !looksJL {
		for _, h := range resourceHosts {
			host := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(h), "https://"), "http://")
			if host != "" && strings.Contains(low, host) {
				looksJL = true
				break
			}
		}
	}
	m := reJLHash.FindString(low)
	if m == "" {
		return "", false
	}
	return m, looksJL
}

type jlResolved struct {
	title   string
	season  int
	episode int
}

// ResolveByHash 通过资源站分享页把内容标识解析为 剧名/季/集。
func (r *JuliangResolver) ResolveByHash(hash string) (string, int, int, error) {
	cfg := r.store.Get()
	key := "jl:" + hash
	if v, ok := r.cache.get(key); ok {
		rr := v.(jlResolved)
		return rr.title, rr.season, rr.episode, nil
	}

	var boot *ShareBoot
	overall, cancelOverall := context.WithTimeout(context.Background(),
		cfg.HTTPTimeout*time.Duration(2*len(cfg.ResourceHosts))+30*time.Second)
	defer cancelOverall()
	for _, host := range cfg.ResourceHosts {
		u := strings.TrimRight(host, "/") + "/s/" + hash
		body, _, err := httpGetWithRetry(overall, r.client, u, nil, 2, cfg.HTTPTimeout)
		if err != nil {
			continue
		}
		m := reShareBoot.FindSubmatch(body)
		if m == nil {
			continue
		}
		var b ShareBoot
		if err := json.Unmarshal(m[1], &b); err != nil {
			continue
		}
		if strings.TrimSpace(b.ContentTitle) != "" {
			boot = &b
			break
		}
	}
	if boot == nil {
		return "", 0, 0, fmt.Errorf("resource share page unavailable for hash %s", hash)
	}

	season, episode := 1, 1
	for _, u := range boot.Navigation {
		if u.UnitID == boot.Unit {
			if u.SeasonNumber > 0 {
				season = u.SeasonNumber
			}
			if u.EpisodeNumber > 0 {
				episode = u.EpisodeNumber
			}
			break
		}
	}
	r.cache.set(key, jlResolved{boot.ContentTitle, season, episode}, cfg.ResolveTTL)
	return boot.ContentTitle, season, episode, nil
}

// ---- 豆瓣 ID 兜底 ----

type DoubanDetail struct {
	Title string `json:"title"`
	Type  string `json:"type"`
	Year  string `json:"year"`
}

// ResolveDouban 按豆瓣 ID 取标题与类型（type: movie/tv）。
func (r *JuliangResolver) ResolveDouban(id string) (*DoubanDetail, error) {
	cfg := r.store.Get()
	u := "https://m.douban.com/rexxar/api/v2/movie/" + url.PathEscape(id) + "?for_mobile=1"
	headers := map[string]string{"Referer": "https://m.douban.com/movie/"}
	overall, cancel := context.WithTimeout(context.Background(), cfg.HTTPTimeout*3+15*time.Second)
	defer cancel()
	body, status, err := httpGetWithRetry(overall, r.client, u, headers, 3, cfg.HTTPTimeout)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("douban returned status %d", status)
	}
	var d DoubanDetail
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	if strings.TrimSpace(d.Title) == "" {
		return nil, fmt.Errorf("douban subject %s has no title", id)
	}
	return &d, nil
}
