//go:build smoke

// 真实站点端到端冒烟测试（详情页 → 播放页 → m3u8 → 弹幕匹配 → HTTP 接口）。
//
// 默认不参与普通构建，需显式开启，且必须设置 CMS_SMOKE=1 才会真正访问外网：
//
//	CMS_SMOKE=1 go test -tags smoke -run TestCMSSmoke -v .
//
// 可选环境变量：
//
//	CMS_SMOKE_BASE   站点根地址，默认 https://www.501710491.xyz
//	CMS_SMOKE_ID     视频 ID，默认 2421
package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCMSSmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv("CMS_SMOKE")) != "1" {
		t.Skip("未设置 CMS_SMOKE=1，跳过真实站点冒烟")
	}

	base := strings.TrimSpace(os.Getenv("CMS_SMOKE_BASE"))
	if base == "" {
		base = "https://www.501710491.xyz"
	}
	id := strings.TrimSpace(os.Getenv("CMS_SMOKE_ID"))
	if id == "" {
		id = "2421"
	}

	cfg := defaultConfig()
	cfg.CMSBaseURL = base
	cfg.CMSTimeout = 30 * time.Second
	cfg.CMSConcurrency = 3
	cfg.CMSAllEpisodes = false
	cfg.CMSMaxEpisodes = 3
	if len(cfg.ResourceHosts) == 0 {
		cfg.ResourceHosts = []string{"https://jimaoys95.com", "https://jimaoys94.com"}
	}

	store := NewConfigStore("")
	store.Set(cfg)

	s := &Server{
		store: store,
		lv:    NewLogVarClient(store),
		jl:    NewJuliangResolver(store),
		sm:    NewSourceManager(store),
		cms:   NewCMSCrawler(store),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	detail, err := s.cms.ResolveDetail(ctx, cfg.CMSBaseURL, id, "")
	if err != nil {
		t.Fatalf("ResolveDetail 失败：%v", err)
	}
	t.Logf("详情页解析：title=%q sources=%d", detail.Title, len(detail.Sources))
	for _, src := range detail.Sources {
		t.Logf("  sid=%d from=%s name=%s episodes=%d", src.SID, src.From, src.Name, len(src.Episodes))
	}

	eps := s.cms.FetchM3U8(ctx, detail, 0, 0, false)
	for i := range eps {
		e := &eps[i]
		t.Logf("  play: sid=%d nid=%d ok=%v encrypt=%d m3u8=%s error=%s", e.SID, e.NID, e.OK, e.Encrypt, e.M3U8, e.Err)
	}

	first := -1
	for i := range eps {
		if eps[i].OK {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatalf("没有抓到任何可用 m3u8")
	}

	baseTitle, season, _ := parseAnimeTitle(detail.Title)
	cs, name, err := s.matchOneEpisode(ctx, cfg, detail, eps[first], baseTitle, season)
	t.Logf("弹幕匹配：name=%s danmu=%d err=%v", name, len(cs), err)

	// 接口层验证：?ac=cms&id={id}
	req := httptest.NewRequest("GET", "/cms?id="+id+"&nid="+strconv.Itoa(eps[first].NID), nil)
	w := httptest.NewRecorder()
	s.handleDanmu(w, req)

	body := w.Body.String()
	var resp struct {
		Code  int    `json:"code"`
		Name  string `json:"name"`
		Danum int    `json:"danum"`
		Msg   string `json:"msg"`
		CMS   struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			OKCount  int    `json:"ok_count"`
			Total    int    `json:"total"`
			Episodes []struct {
				SID   int    `json:"sid"`
				NID   int    `json:"nid"`
				M3U8  string `json:"m3u8"`
				OK    bool   `json:"ok"`
				Danum int    `json:"danum"`
			} `json:"episodes"`
		} `json:"cms"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("响应不是合法 JSON：%v body=%.500s", err, body)
	}
	t.Logf("接口返回：code=%d name=%q danum=%d msg=%q ok=%d/%d", resp.Code, resp.Name, resp.Danum, resp.Msg, resp.CMS.OKCount, resp.CMS.Total)
	for _, e := range resp.CMS.Episodes {
		t.Logf("  api ep: sid=%d nid=%d ok=%v danum=%d m3u8=%s", e.SID, e.NID, e.OK, e.Danum, e.M3U8)
	}
	if resp.Code != 1 {
		t.Fatalf("接口返回失败：%s", resp.Msg)
	}
}
