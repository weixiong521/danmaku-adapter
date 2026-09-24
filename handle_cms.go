package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
)

// handleCMS 处理 ?ac=cms 请求。
//
// 入参（GET）：
//
//	id            视频 ID，如 2421
//	url           视频详情页 URL（与 id 二选一；传播放页地址亦可，会自动提取 ID）
//	sid           可选，仅处理指定播放源
//	nid           可选，仅处理指定集
//	all           可选，1/0，覆盖配置 cms_all_episodes：是否抓取全部集
//	match_all     可选，1/0，是否把所有集的弹幕合并返回（默认仅返回“选中集”的弹幕）
//	danmu         可选，0 表示只抓 m3u8 不做弹幕匹配（默认 1）
//
// 返回沿用现有协议（code/name/danum/danmuku），并额外附带 cms 明细节点。
func (s *Server) handleCMS(w http.ResponseWriter, r *http.Request, cfg *Config) {
	q := r.URL.Query()

	input := strings.TrimSpace(firstNonEmpty(q.Get("id"), q.Get("url"), q.Get("detail_url"), q.Get("cms_url")))
	if input == "" {
		s.writeJSON(w, map[string]any{
			"code": 0,
			"msg":  "缺少入参：请使用 ?ac=cms&id={视频ID} 或 ?ac=cms&url={详情页URL}",
		})
		return
	}

	base, id, detailURL := s.cms.parseCMSInput(input)
	if id == "" {
		s.writeJSON(w, map[string]any{
			"code": 0,
			"msg":  "无法从入参解析出视频 ID：" + input,
		})
		return
	}

	sid := parseLeadingInt(q.Get("sid"))
	nid := parseLeadingInt(q.Get("nid"))
	all := cfg.CMSAllEpisodes
	if v := strings.TrimSpace(q.Get("all")); v != "" {
		all = parseBool(v)
	}
	if nid > 0 {
		all = false // 指定了集号就只抓这一集
	}
	matchAll := parseBool(q.Get("match_all"))
	withDanmu := true
	if v := strings.TrimSpace(q.Get("danmu")); v != "" {
		withDanmu = parseBool(v)
	}

	ctx, cancel := context.WithTimeout(r.Context(), cmsOverallTimeout(cfg))
	defer cancel()

	detail, err := s.cms.ResolveDetail(ctx, base, id, detailURL)
	if err != nil {
		log.Printf("[cms] 详情页解析失败：id=%s err=%v", id, err)
		s.writeJSON(w, map[string]any{"code": 0, "msg": err.Error(), "id": id})
		return
	}

	eps := s.cms.FetchM3U8(ctx, detail, sid, nid, all)

	// 选出“顶层弹幕”对应的集：match_all 时合并全部，否则优先命中 nid，再退化为首个成功集。
	matchIdx := make([]int, 0, len(eps))
	if matchAll {
		for i := range eps {
			if eps[i].OK {
				matchIdx = append(matchIdx, i)
			}
		}
	} else {
		pick := -1
		for i := range eps {
			if !eps[i].OK {
				continue
			}
			if nid > 0 && eps[i].NID == nid {
				pick = i
				break
			}
			if pick < 0 {
				pick = i
			}
		}
		if pick >= 0 {
			matchIdx = append(matchIdx, pick)
		}
	}

	var comments []Comment
	danmuName := ""
	if withDanmu && len(matchIdx) > 0 {
		comments = s.matchDanmu(ctx, cfg, detail, eps, matchIdx)
		for _, i := range matchIdx {
			if eps[i].Danum > 0 {
				danmuName = firstNonEmpty(eps[i].DanmuName, eps[i].Name)
				break
			}
		}
	}

	if comments == nil {
		comments = []Comment{}
	}

	episodes := make([]map[string]any, 0, len(eps))
	okCount := 0
	for i := range eps {
		e := &eps[i]
		if e.OK {
			okCount++
		}
		item := map[string]any{
			"sid":      e.SID,
			"nid":      e.NID,
			"name":     e.Name,
			"m3u8":     e.M3U8,
			"ok":       e.OK,
			"encrypt":  e.Encrypt,
			"page_url": e.PageURL,
			"danum":    e.Danum,
		}
		if e.SIDName != "" {
			item["sid_name"] = e.SIDName
		}
		if e.From != "" {
			item["from"] = e.From
		}
		if e.URLNext != "" {
			item["url_next"] = e.URLNext
		}
		if e.DanmuName != "" {
			item["danmu_name"] = e.DanmuName
		}
		if e.Err != "" {
			item["error"] = e.Err
		}
		episodes = append(episodes, item)
	}

	sources := make([]map[string]any, 0, len(detail.Sources))
	for _, src := range detail.Sources {
		sources = append(sources, map[string]any{
			"sid":      src.SID,
			"from":     src.From,
			"name":     src.Name,
			"episodes": len(src.Episodes),
		})
	}

	s.writeJSON(w, map[string]any{
		"code":    1,
		"name":    detail.Title,
		"danum":   len(comments),
		"danmuku": buildDanmuku(comments, cfg, nowUnix()),
		"cms": map[string]any{
			"id":         detail.ID,
			"title":      detail.Title,
			"base":       detail.Base,
			"detail_url": detail.DetailURL,
			"sources":    sources,
			"episodes":   episodes,
			"ok_count":   okCount,
			"total":      len(eps),
			"danmu_name": danmuName,
		},
	})
}

// matchDanmu 为指定集匹配弹幕：
//  1. 若 m3u8 中能提取资源站内容标识，则走“分享页 -> 剧名/季/集”链路（最准确）；
//  2. 否则退化为“详情页剧名 + 集号”链路。
//
// 并发受 cms_concurrency 限制，单集失败不影响其它集。
func (s *Server) matchDanmu(ctx context.Context, cfg *Config, d *CMSDetail, eps []CMSEpisodeResult, idx []int) []Comment {
	baseTitle, titleSeason, _ := parseAnimeTitle(d.Title)
	if strings.TrimSpace(baseTitle) == "" {
		baseTitle = d.Title
	}
	if titleSeason <= 0 {
		titleSeason = 1
	}

	conc := cfg.CMSConcurrency
	if conc <= 0 {
		conc = 1
	}
	if conc > len(idx) {
		conc = len(idx)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	all := make([]Comment, 0, 256)
	sem := make(chan struct{}, conc)

	for _, i := range idx {
		i := i
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			cs, name, err := s.matchOneEpisode(ctx, cfg, d, eps[i], baseTitle, titleSeason)
			mu.Lock()
			eps[i].Danum = len(cs)
			eps[i].DanmuName = name
			all = append(all, cs...)
			mu.Unlock()
			if err != nil {
				log.Printf("[cms] 第%d集弹幕匹配失败：%v", eps[i].NID, err)
			}
		}()
	}
	wg.Wait()
	return all
}

// matchOneEpisode 单集弹幕匹配：
//  1. 优先用 m3u8 里的资源站内容标识反查“剧名/季/集”（最准确）；
//  2. 资源站链路拿不到弹幕时，回退到“详情页剧名 + 集号”，避免单条链路失败导致整集无弹幕。
func (s *Server) matchOneEpisode(ctx context.Context, cfg *Config, d *CMSDetail, ep CMSEpisodeResult, baseTitle string, titleSeason int) ([]Comment, string, error) {
	if hash, ok := ExtractHash(ep.M3U8, cfg.ResourceHosts); ok && hash != "" {
		if title, season, episode, err := s.jl.ResolveByHash(hash); err == nil && strings.TrimSpace(title) != "" {
			if cs, name, err := s.lv.ResolveByTitle(title, season, episode); err == nil && len(cs) > 0 {
				return cs, firstNonEmpty(name, title), nil
			} else if err != nil {
				log.Printf("[cms] 资源站剧名 %q 未匹配到弹幕，回退详情页剧名 %q：%v", title, baseTitle, err)
			}
		}
	}
	cs, name, err := s.lv.ResolveByTitle(baseTitle, titleSeason, ep.NID)
	return cs, firstNonEmpty(name, baseTitle), err
}
