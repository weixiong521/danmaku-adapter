package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---- 苹果CMS（MacCMS V10）直连抓取 ----
//
// 目标：给定视频 ID 或详情页 URL，先解析详情页拿到全部 sid/nid，
// 再逐集访问播放页提取内联脚本 player_aaaa，取到真实 m3u8 播放地址，
// 最后把 m3u8 交给现有弹幕链路（LogVar）匹配弹幕。
//
// 抓取链路（以苹果CMS V10 默认路由为例）：
//
//	GET /index.php/vod/detail/id/{id}.html        -> 全部分集链接（sid / nid）
//	GET /index.php/vod/play/id/{id}/sid/{s}/nid/{n}.html
//	     -> var player_aaaa = {...};  正则 + JSON 解析
//	     -> encrypt == 0 -> url 即真实 m3u8
//	     -> encrypt == 1 -> 交由 cmsDecryptHook 预留钩子处理

var (
	// 详情页内每条分集链接：/index.php/vod/play/id/{id}/sid/{sid}/nid/{nid}.html
	reCMSPlayLink = regexp.MustCompile(`/vod/play/id/(\d+)/sid/(\d+)/nid/(\d+)\.html`)
	// 同上，但额外抓取紧随其后的集名文本（用于展示“第 N 集”名称）
	reCMSPlayLinkName = regexp.MustCompile(`(?s)/vod/play/id/(?:\d+)/sid/(\d+)/nid/(\d+)\.html["'][^>]*>(?:\s*<[^>]*>)*\s*([^<]{1,64})`)
	// 播放页内联播放信息：var player_aaaa = {...};（仅定位对象起点，内容用括号配平提取）
	reCMSPlayerAAAA = regexp.MustCompile(`(?s)var\s+player_aaaa\s*=\s*(\{)`)
	// 站点播放源配置：MacPlayerConfig.player_list = {...} 或 JSON 中的 "player_list": {...}
	reCMSPlayerList = regexp.MustCompile(`(?s)(?:MacPlayerConfig\.player_list\s*=\s*|["']player_list["']\s*:\s*)(\{)`)
	reCMSTitleTag   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	// 详情页主标题（比 <title> 干净，站点常把分类/后缀拼进 <title>）
	reCMSH1      = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	reCMSHTMLTag = regexp.MustCompile(`(?s)<[^>]*>`)
	reCMSIDPath  = regexp.MustCompile(`(?i)/id/(\d+)`)
	reCMSIDQuery = regexp.MustCompile(`(?i)[?&]id=(\d+)`)
	reCMSTailNum = regexp.MustCompile(`(\d+)(?:\.html?)?$`)
	reCMSPlainID = regexp.MustCompile(`^\d+$`)
	reCMSSpaces  = regexp.MustCompile(`\s+`)
)

// flexInt 兼容 JSON 中“数字”与“字符串数字”两种写法（部分站点把 encrypt/sid/nid 写成字符串）。
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

// cmsPlayerAAAA 对应播放页内联脚本 var player_aaaa = {...};。
type cmsPlayerAAAA struct {
	Flag     string  `json:"flag"`
	Encrypt  flexInt `json:"encrypt"` // 0=明文；1=MacPlayer 加密，需 cmsDecryptHook
	URL      string  `json:"url"`
	URLNext  string  `json:"url_next"`
	From     string  `json:"from"`
	Server   string  `json:"server"`
	ID       string  `json:"id"`
	SID      flexInt `json:"sid"`
	NID      flexInt `json:"nid"`
	LinkNext string  `json:"link_next"`
}

// cmsPlayerEntry 对应 MacPlayerConfig.player_list 里的单个播放源配置。
type cmsPlayerEntry struct {
	Show  string `json:"show"`
	From  string `json:"from"`
	Parse string `json:"parse"`
	PS    string `json:"ps"`
}

// CMSEpisodeRef 是详情页解析出的一条分集引用。
type CMSEpisodeRef struct {
	SID  int    // 播放源序号
	NID  int    // 集号
	Name string // 集名（如“第 2 季 第 1 集”）
}

// CMSSourceRef 是一个播放源（sid）及其全部分集。
type CMSSourceRef struct {
	SID      int
	From     string // 播放源标识（如 jlm3u8），可能为空
	Name     string // 播放源显示名（如“巨量资源”），可能为空
	Episodes []CMSEpisodeRef
}

// CMSDetail 是详情页解析结果。
type CMSDetail struct {
	ID        string
	Title     string
	Base      string // 站点根地址
	DetailURL string
	Sources   []CMSSourceRef
}

// CMSEpisodeResult 是单集抓取结果。
type CMSEpisodeResult struct {
	SID       int
	SIDName   string
	From      string
	NID       int
	Name      string
	PageURL   string
	M3U8      string
	URLNext   string
	Encrypt   int
	OK        bool
	Err       string
	Danum     int
	DanmuName string
}

// CMSCrawler 苹果CMS 直连抓取器。
// 与其它组件一样只持有 ConfigStore，配置热更新即时生效。
type CMSCrawler struct {
	store  *ConfigStore
	client *http.Client
	cache  *ttlCache
}

func NewCMSCrawler(store *ConfigStore) *CMSCrawler {
	return &CMSCrawler{store: store, client: &http.Client{}, cache: newTTLCache()}
}

// normalizeCMSBase 规范化站点根地址：补协议、去尾斜杠、host/scheme 转小写、保留可能的子路径。
func normalizeCMSBase(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return strings.TrimRight(strings.ToLower(s), "/")
	}
	base := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
	if p := strings.TrimRight(u.Path, "/"); p != "" {
		base += p
	}
	return base
}

// extractCMSID 从 ID 字符串或详情页/播放页地址中提取视频 ID。
func extractCMSID(s string) string {
	if reCMSPlainID.MatchString(strings.TrimSpace(s)) {
		return strings.TrimSpace(s)
	}
	if m := reCMSIDPath.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := reCMSIDQuery.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := reCMSTailNum.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// parseCMSInput 解析调用方传入的 id / url，返回站点根地址、视频 ID、详情页地址。
// 说明：未指定站点时使用配置项 cms_base_url；传入完整地址时会自动识别站点域名与 ID，
// 若传入的是播放页地址，则详情页地址由站点域名 + ID 重新拼装。
func (c *CMSCrawler) parseCMSInput(raw string) (base, id, detailURL string) {
	raw = strings.TrimSpace(raw)
	cfg := c.store.Get()
	base = normalizeCMSBase(cfg.CMSBaseURL)
	if raw == "" {
		return base, "", ""
	}
	low := strings.ToLower(raw)

	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return base, extractCMSID(raw), ""
	}

	// 完整 URL：优先采用地址里的站点（支持多站点共用一套服务）
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		b := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
		if idx := strings.Index(strings.ToLower(u.Path), "/index.php"); idx > 0 {
			b += strings.TrimRight(u.Path[:idx], "/")
		}
		base = b
	}
	id = extractCMSID(raw)
	if !strings.Contains(low, "/vod/play/") {
		detailURL = raw // 本身就是详情页地址
	}
	return base, id, detailURL
}

func cmsDetailURL(base, id string) string {
	return strings.TrimRight(base, "/") + "/index.php/vod/detail/id/" + id + ".html"
}

func cmsPlayURL(base, id string, sid, nid int) string {
	return fmt.Sprintf("%s/index.php/vod/play/id/%s/sid/%d/nid/%d.html",
		strings.TrimRight(base, "/"), id, sid, nid)
}

// cmsHeaders 抓取详情页/播放页时携带的请求头。
// 苹果CMS 站点多挂在 Cloudflare 之后，缺少浏览器常规头容易被判定为爬虫而返回 5xx，
// 因此除 Referer 外还补齐 Accept / Accept-Language / Sec-Fetch-* 等常规字段。
func cmsHeaders(base string) map[string]string {
	h := map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		"Accept-Language":           "zh-CN,zh;q=0.9,en;q=0.8",
		"Upgrade-Insecure-Requests": "1",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "same-origin",
		"Sec-Fetch-User":            "?1",
	}
	if b := strings.TrimRight(strings.TrimSpace(base), "/"); b != "" {
		h["Referer"] = b + "/"
	}
	return h
}

// normalizeCMSText 去掉 HTML 标签并压缩空白。
func normalizeCMSText(s string) string {
	t := reCMSHTMLTag.ReplaceAllString(s, " ")
	t = reCMSSpaces.ReplaceAllString(t, " ")
	return strings.TrimSpace(t)
}

// cleanCMSTitle 提取剧名：优先取详情页 <h1>；退化再从 <title> 剥离站点后缀。
// 例：<title>灵猪降妖_电影__视频详情-其他 - 免费短剧…</title> -> "灵猪降妖"。
func cleanCMSTitle(html string) string {
	if m := reCMSH1.FindStringSubmatch(html); m != nil {
		if t := normalizeCMSText(m[1]); t != "" && !strings.Contains(t, "视频详情") && len([]rune(t)) <= 60 {
			return t
		}
	}
	m := reCMSTitleTag.FindStringSubmatch(html)
	if m == nil {
		return ""
	}
	t := normalizeCMSText(m[1])
	// 先剥掉站点名后缀（" - 站点名" / " | 站点名"）
	for _, sep := range []string{" - ", " | ", "—", "|"} {
		if i := strings.Index(t, sep); i > 0 {
			if head := strings.TrimSpace(t[:i]); head != "" {
				t = head
			}
			break
		}
	}
	// 再剥掉站点拼进标题的分类/详情尾巴（"剧名_电影__视频详情-其他" -> "剧名"）
	if i := strings.Index(t, "_"); i > 0 {
		if head := strings.TrimSpace(t[:i]); head != "" {
			t = head
		}
	}
	return strings.TrimSpace(t)
}

// extractBalancedObject 从 s[start]（必须是 '{'）开始做括号配平，返回完整的 JSON 对象文本。
// 字符串字面量中的花括号与转义会被正确跳过，因此嵌套对象不会被截断。
func extractBalancedObject(s string, start int) (string, bool) {
	if start < 0 || start >= len(s) || s[start] != '{' {
		return "", false
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

// parsePlayerAAAA 从播放页 HTML 中提取并解析 var player_aaaa = {...};。
func parsePlayerAAAA(html string) (*cmsPlayerAAAA, error) {
	loc := reCMSPlayerAAAA.FindStringSubmatchIndex(html)
	if loc == nil {
		return nil, fmt.Errorf("播放页未找到内联脚本 player_aaaa")
	}
	obj, ok := extractBalancedObject(html, loc[2])
	if !ok {
		return nil, fmt.Errorf("player_aaaa 结构不完整（花括号未配平）")
	}
	var pa cmsPlayerAAAA
	if err := json.Unmarshal([]byte(obj), &pa); err != nil {
		return nil, fmt.Errorf("player_aaaa 解析失败：%w", err)
	}
	return &pa, nil
}

// parseCMSPlayerList 解析 MacPlayerConfig.player_list（失败返回 nil，不影响主流程）。
func parseCMSPlayerList(html string) map[string]cmsPlayerEntry {
	loc := reCMSPlayerList.FindStringSubmatchIndex(html)
	if loc == nil {
		return nil
	}
	obj, ok := extractBalancedObject(html, loc[2])
	if !ok {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(obj), &raw); err != nil {
		return nil
	}
	out := make(map[string]cmsPlayerEntry, len(raw))
	for k, v := range raw {
		var e cmsPlayerEntry
		if err := json.Unmarshal(v, &e); err != nil {
			out[k] = cmsPlayerEntry{}
			continue
		}
		out[k] = e
	}
	return out
}

// parseCMSSources 从详情页 HTML 解析全部播放源与分集（按 sid、nid 排序）。
func parseCMSSources(html string, players map[string]cmsPlayerEntry) []CMSSourceRef {
	names := map[string]string{} // "sid:nid" -> 集名
	for _, m := range reCMSPlayLinkName.FindAllStringSubmatch(html, -1) {
		sid, _ := strconv.Atoi(m[1])
		nid, _ := strconv.Atoi(m[2])
		name := strings.TrimSpace(reCMSHTMLTag.ReplaceAllString(m[3], ""))
		name = strings.TrimSpace(reCMSSpaces.ReplaceAllString(name, " "))
		if name != "" {
			names[strconv.Itoa(sid)+":"+strconv.Itoa(nid)] = name
		}
	}

	order := make([]int, 0, 4)
	seenSID := map[int]bool{}
	seenEp := map[string]bool{}
	bySID := map[int][]CMSEpisodeRef{}
	for _, m := range reCMSPlayLink.FindAllStringSubmatch(html, -1) {
		sid, _ := strconv.Atoi(m[2])
		nid, _ := strconv.Atoi(m[3])
		key := strconv.Itoa(sid) + ":" + strconv.Itoa(nid)
		if seenEp[key] {
			continue
		}
		seenEp[key] = true
		if !seenSID[sid] {
			seenSID[sid] = true
			order = append(order, sid)
		}
		name := names[key]
		if name == "" {
			name = "第 " + strconv.Itoa(nid) + " 集"
		}
		bySID[sid] = append(bySID[sid], CMSEpisodeRef{SID: sid, NID: nid, Name: name})
	}
	sort.Ints(order)

	// 播放源显示名：仅当 player_list 的条目数与详情页源数一致时按顺序对应（避免错配）
	flags := make([]string, 0, len(players))
	for k := range players {
		flags = append(flags, k)
	}
	sort.Strings(flags)
	matchFlags := len(order) > 0 && len(flags) == len(order)

	out := make([]CMSSourceRef, 0, len(order))
	for i, sid := range order {
		eps := bySID[sid]
		sort.Slice(eps, func(a, b int) bool { return eps[a].NID < eps[b].NID })
		src := CMSSourceRef{SID: sid, Episodes: eps}
		if matchFlags {
			flag := flags[i]
			src.From = flag
			if e, ok := players[flag]; ok {
				src.Name = strings.TrimSpace(e.Show)
			}
		}
		// 单个源时即便数量对不上也尽量给出名字
		if src.Name == "" && len(order) == 1 && len(flags) == 1 {
			src.From = flags[0]
			src.Name = strings.TrimSpace(players[flags[0]].Show)
		}
		out = append(out, src)
	}
	return out
}

// ResolveDetail 抓取并解析详情页：得到剧名与全部 sid/nid 列表（带 TTL 缓存）。
func (c *CMSCrawler) ResolveDetail(ctx context.Context, base, id, detailURL string) (*CMSDetail, error) {
	cfg := c.store.Get()
	if id == "" {
		return nil, fmt.Errorf("缺少视频 id：请使用 ?ac=cms&id=2421 或 ?ac=cms&url=<详情页地址>")
	}
	if base == "" {
		base = normalizeCMSBase(cfg.CMSBaseURL)
	}
	if detailURL == "" {
		if base == "" {
			return nil, fmt.Errorf("未配置站点域名（cms_base_url），且入参未提供完整详情页地址")
		}
		detailURL = cmsDetailURL(base, id)
	}

	cacheKey := "cms:detail:" + base + "|" + id + "|" + detailURL
	if v, ok := c.cache.get(cacheKey); ok {
		return v.(*CMSDetail), nil
	}

	body, status, err := httpGetWithRetry(ctx, c.client, detailURL, cmsHeaders(base), 2, cfg.CMSTimeout)
	if err != nil {
		return nil, fmt.Errorf("详情页请求失败：%w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("详情页返回状态 %d", status)
	}
	html := string(body)

	d := &CMSDetail{
		ID:        id,
		Base:      base,
		DetailURL: detailURL,
		Title:     cleanCMSTitle(html),
		Sources:   parseCMSSources(html, parseCMSPlayerList(html)),
	}
	if len(d.Sources) == 0 {
		return nil, fmt.Errorf("详情页未解析到任何分集链接（id=%s url=%s）", id, detailURL)
	}
	if d.Title == "" {
		d.Title = "视频" + id
	}
	c.cache.set(cacheKey, d, cfg.ResolveTTL)
	log.Printf("[cms] 详情页解析完成：id=%s 剧名=%q 播放源=%d 集数=%d",
		id, d.Title, len(d.Sources), countCMSEpisodes(d.Sources))
	return d, nil
}

func countCMSEpisodes(sources []CMSSourceRef) int {
	n := 0
	for _, s := range sources {
		n += len(s.Episodes)
	}
	return n
}

// fetchEpisode 抓取单个播放页并提取真实 m3u8。
func (c *CMSCrawler) fetchEpisode(ctx context.Context, base, id string, ref CMSEpisodeRef, from, sidName string) CMSEpisodeResult {
	cfg := c.store.Get()
	res := CMSEpisodeResult{
		SID:     ref.SID,
		SIDName: sidName,
		From:    from,
		NID:     ref.NID,
		Name:    ref.Name,
		PageURL: cmsPlayURL(base, id, ref.SID, ref.NID),
	}

	body, status, err := httpGetWithRetry(ctx, c.client, res.PageURL, cmsHeaders(base), 2, cfg.CMSTimeout)
	if err != nil {
		res.Err = "播放页请求失败：" + err.Error()
		return res
	}
	if status != http.StatusOK {
		res.Err = fmt.Sprintf("播放页返回状态 %d", status)
		return res
	}

	pa, err := parsePlayerAAAA(string(body))
	if err != nil {
		res.Err = err.Error()
		return res
	}
	res.Encrypt = int(pa.Encrypt)
	res.URLNext = pa.URLNext
	if strings.TrimSpace(pa.URL) == "" {
		res.Err = "player_aaaa 未包含播放地址"
		return res
	}

	if res.Encrypt == 0 {
		res.M3U8 = strings.TrimSpace(pa.URL)
	} else {
		plain, derr := cmsDecryptHook(strings.TrimSpace(pa.URL))
		if derr != nil {
			res.Err = derr.Error()
			return res
		}
		res.M3U8 = plain
	}

	if !strings.HasPrefix(strings.ToLower(res.M3U8), "http") {
		res.Err = "播放地址不是有效 URL：" + res.M3U8
		res.M3U8 = ""
		return res
	}
	res.OK = true
	return res
}

// cmsDecryptHook 预留的 MacPlayer 加密地址解密钩子。
//
// 苹果CMS 的 MacPlayer 前端对 encrypt=1 的 url 使用动态密钥 + AES/CBC 解密，
// 各站点实现不一（密钥随站点配置下发），因此这里只做“常见弱编码”尝试，
// 失败时返回明确错误。需要支持某个具体站点时，在此函数内补充该站点的解密逻辑即可。
func cmsDecryptHook(encrypted string) (string, error) {
	if s, err := url.QueryUnescape(encrypted); err == nil && strings.Contains(s, ".m3u8") {
		return s, nil
	}
	if s, err := decodeBase64Loose(encrypted); err == nil && strings.Contains(s, ".m3u8") {
		return s, nil
	}
	return "", fmt.Errorf("该集播放地址为 MacPlayer 加密串（encrypt=1），当前仅预留解密钩子 cmsDecryptHook：请按站点补充解密逻辑")
}

// decodeBase64Loose 依次尝试标准 / URL 安全 / 无填充 Base64 解码。
func decodeBase64Loose(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty")
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("not base64")
}

// FetchM3U8 并发逐集抓取播放页。
// all=true 抓取（过滤后）全部集；all=false 时只抓指定 nid，nid 为 0 则每个源只抓第一集。
func (c *CMSCrawler) FetchM3U8(ctx context.Context, d *CMSDetail, sid, nid int, all bool) []CMSEpisodeResult {
	cfg := c.store.Get()

	type task struct {
		ref     CMSEpisodeRef
		from    string
		sidName string
	}
	tasks := make([]task, 0, countCMSEpisodes(d.Sources))
	for _, src := range d.Sources {
		if sid > 0 && src.SID != sid {
			continue
		}
		if all {
			for _, ep := range src.Episodes {
				tasks = append(tasks, task{ref: ep, from: src.From, sidName: src.Name})
			}
			continue
		}
		if nid > 0 {
			for _, ep := range src.Episodes {
				if ep.NID == nid {
					tasks = append(tasks, task{ref: ep, from: src.From, sidName: src.Name})
					break
				}
			}
			continue
		}
		if len(src.Episodes) > 0 {
			ep := src.Episodes[0]
			tasks = append(tasks, task{ref: ep, from: src.From, sidName: src.Name})
		}
	}
	if limit := cfg.CMSMaxEpisodes; limit > 0 && len(tasks) > limit {
		log.Printf("[cms] 任务数 %d 超过上限 %d，已截断", len(tasks), limit)
		tasks = tasks[:limit]
	}

	results := make([]CMSEpisodeResult, len(tasks))
	if len(tasks) == 0 {
		return results
	}

	conc := cfg.CMSConcurrency
	if conc <= 0 {
		conc = 1
	}
	if conc > len(tasks) {
		conc = len(tasks)
	}

	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i := range tasks {
		i := i
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = c.fetchEpisode(ctx, d.Base, d.ID, tasks[i].ref, tasks[i].from, tasks[i].sidName)
		}()
	}
	wg.Wait()

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].SID != results[j].SID {
			return results[i].SID < results[j].SID
		}
		return results[i].NID < results[j].NID
	})

	ok := 0
	for i := range results {
		if results[i].OK {
			ok++
		}
	}
	log.Printf("[cms] 抓取完成：id=%s 请求=%d 成功=%d", d.ID, len(results), ok)
	return results
}

// cmsOverallTimeout 单次 ?ac=cms 请求的整体超时（受 cms_timeout_ms 与集数影响，设上下限兜底）。
func cmsOverallTimeout(cfg *Config) time.Duration {
	t := cfg.CMSTimeout * 4
	if t < 30*time.Second {
		t = 30 * time.Second
	}
	if t > 3*time.Minute {
		t = 3 * time.Minute
	}
	return t
}
