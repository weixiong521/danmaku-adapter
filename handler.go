package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// 官方平台地址特征；命中后直接按 URL 取弹幕。
var platformMarkers = []string{
	".qq.com", ".iqiyi.com", ".mgtv.com", ".bilibili.com", "b23.tv",
	".youku.com", ".miguvideo.com", ".sohu.com", "www.le.com",
	".douyin.com", ".ixigua.com", ".mddcloud.com.cn", ".yfsp.tv",
}

func isPlatformURL(u string) bool {
	low := strings.ToLower(u)
	for _, m := range platformMarkers {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type Server struct {
	store *ConfigStore
	lv    *LogVarClient
	jl    *JuliangResolver
	sm    *SourceManager
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[http] encode error: %v", err)
	}
}

func (s *Server) handleDanmu(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	cfg := s.store.Get()

	q := r.URL.Query()
	inURL := q.Get("url")
	doubanID := strings.TrimSpace(q.Get("douban_id"))
	extraEp := parseLeadingInt(firstNonEmpty(q.Get("ep"), q.Get("episode"))) // 非 Getapp 默认字段，仅做兜底

	var comments []Comment
	name := ""

	switch {
	case isPlatformURL(inURL):
		log.Printf("[danmu] 官方平台地址，直接取弹幕: %s", inURL)
		cs, err := s.lv.CommentByURL(inURL)
		if err != nil {
			log.Printf("[danmu] 按地址取弹幕失败: %v", err)
		}
		comments, name = cs, inURL

	default:
		if hash, looksJL := ExtractHash(inURL, cfg.ResourceHosts); looksJL {
			title, season, episode, err := s.jl.ResolveByHash(hash)
			if err != nil {
				log.Printf("[danmu] 资源站分享页解析失败: %v", err)
				break
			}
			log.Printf("[danmu] 资源站 -> 剧名=%q 第%d季 第%d集", title, season, episode)
			cs, n, err := s.lv.ResolveByTitle(title, season, episode)
			if err != nil {
				log.Printf("[danmu] 按标题取弹幕失败: %v", err)
			}
			comments, name = cs, firstNonEmpty(n, title)
		} else if doubanID != "" {
			d, err := s.jl.ResolveDouban(doubanID)
			if err != nil {
				log.Printf("[danmu] 豆瓣解析失败: %v", err)
				break
			}
			log.Printf("[danmu] 豆瓣 -> 剧名=%q 类型=%s", d.Title, d.Type)
			if d.Type == "movie" {
				cs, n, err := s.lv.ResolveByTitle(d.Title, 1, 1)
				if err != nil {
					log.Printf("[danmu] 电影取弹幕失败: %v", err)
				}
				comments, name = cs, firstNonEmpty(n, d.Title)
			} else if extraEp > 0 {
				cs, n, err := s.lv.ResolveByTitle(d.Title, 1, extraEp)
				if err != nil {
					log.Printf("[danmu] 剧集取弹幕失败: %v", err)
				}
				comments, name = cs, firstNonEmpty(n, d.Title)
			} else {
				log.Printf("[danmu] 豆瓣剧集但缺少集数信息，无法匹配（Getapp 默认不传集数）")
				name = d.Title
			}
		}
	}

	if comments == nil {
		comments = []Comment{}
	}
	resp := map[string]any{
		"code":    1,
		"name":    name,
		"danum":   len(comments),
		"danmuku": buildDanmuku(comments, cfg, nowUnix()),
	}
	s.writeJSON(w, resp)
}
