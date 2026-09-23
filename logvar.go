package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---- LogVar（弹弹play 兼容）数据结构 ----

type Anime struct {
	AnimeID      int64    `json:"animeId"`
	BangumiID    string   `json:"bangumiId"`
	AnimeTitle   string   `json:"animeTitle"`
	Type         string   `json:"type"`
	EpisodeCount int      `json:"episodeCount"`
	Source       string   `json:"source"`
	Aliases      []string `json:"aliases"`
}

type searchResp struct {
	ErrorCode int     `json:"errorCode"`
	Success   bool    `json:"success"`
	Animes    []Anime `json:"animes"`
}

type Episode struct {
	EpisodeID     json.Number `json:"episodeId"`
	EpisodeNumber string      `json:"episodeNumber"`
	EpisodeTitle  string      `json:"episodeTitle"`
	URL           string      `json:"url"`
}

type bangumiResp struct {
	ErrorCode int `json:"errorCode"`
	Bangumi   *struct {
		Episodes []Episode `json:"episodes"`
	} `json:"bangumi"`
}

type Comment struct {
	Cid int64  `json:"cid"`
	P   string `json:"p"`
	M   string `json:"m"`
}

type commentResp struct {
	Count     int       `json:"count"`
	Comments  []Comment `json:"comments"`
	ErrorCode int       `json:"errorCode"`
	Success   bool      `json:"success"`
	ErrorMsg  string    `json:"errorMessage"`
}

// LogVarClient 持有 ConfigStore，每次请求读取“当前生效配置”，
// 因此上游地址/令牌可被 /admin 接口热替换而无需重启。
type LogVarClient struct {
	store  *ConfigStore
	client *http.Client
	cache  *ttlCache
}

func NewLogVarClient(store *ConfigStore) *LogVarClient {
	// 不设 Client 级超时，超时统一由每次请求的 context 控制（支持热更新）。
	return &LogVarClient{
		store:  store,
		client: &http.Client{},
		cache:  newTTLCache(),
	}
}

// apiPath 拼接 LogVar 路径；若配置了令牌则插入 /{token}。
func (c *LogVarClient) apiPath(p string) string {
	cfg := c.store.Get()
	base := cfg.LogVarBase
	if t := strings.Trim(cfg.LogVarToken, "/"); t != "" {
		base += "/" + t
	}
	return base + p
}

func (c *LogVarClient) getJSON(u string, out any) error {
	cfg := c.store.Get()
	overall, cancel := context.WithTimeout(context.Background(), cfg.HTTPTimeout*4+30*time.Second)
	defer cancel()
	body, status, err := httpGetWithRetry(overall, c.client, u, nil, 3, cfg.HTTPTimeout)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("logvar returned status %d", status)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}

func (c *LogVarClient) Search(keyword string) ([]Anime, error) {
	u := c.apiPath("/api/v2/search/anime?keyword=" + url.QueryEscape(keyword))
	var r searchResp
	if err := c.getJSON(u, &r); err != nil {
		return nil, err
	}
	return r.Animes, nil
}

func (c *LogVarClient) Bangumi(animeID string) ([]Episode, error) {
	u := c.apiPath("/api/v2/bangumi/" + url.PathEscape(animeID))
	var r bangumiResp
	if err := c.getJSON(u, &r); err != nil {
		return nil, err
	}
	if r.Bangumi == nil {
		return nil, fmt.Errorf("bangumi %s not found", animeID)
	}
	return r.Bangumi.Episodes, nil
}

func (c *LogVarClient) Comment(episodeID string) ([]Comment, error) {
	u := c.apiPath("/api/v2/comment/" + url.PathEscape(episodeID))
	var r commentResp
	if err := c.getJSON(u, &r); err != nil {
		return nil, err
	}
	return r.Comments, nil
}

// CommentByURL 直接按播放地址取弹幕（官方平台地址效果最好）。
func (c *LogVarClient) CommentByURL(videoURL string) ([]Comment, error) {
	u := c.apiPath("/api/v2/comment?url=" + url.QueryEscape(videoURL))
	var r commentResp
	if err := c.getJSON(u, &r); err != nil {
		return nil, err
	}
	return r.Comments, nil
}

// score 为候选 anime 打分，决定选哪一部/哪一季/哪个源。
func (c *LogVarClient) score(a *Anime, wantTitle string, wantSeason, wantEp int) float64 {
	base, season, _ := parseAnimeTitle(a.AnimeTitle)
	s := 0.0
	if titleEqual(base, wantTitle) {
		s += 100
	} else if titleSimilar(base, wantTitle) {
		s += 60
	} else {
		s -= 60
	}
	if season == wantSeason {
		s += 40
	} else {
		s -= 20
	}
	if strings.EqualFold(a.Source, "vod") {
		s -= 30 // vod 源的 m3u8 通常取不到弹幕，降权
	}
	for i, p := range c.store.Get().SourcePriority {
		if strings.EqualFold(a.Source, p) {
			s += 25 - float64(i)*1.5
			break
		}
	}
	if a.EpisodeCount >= wantEp {
		s += 5
	} else {
		s -= 10
	}
	return s
}

// pickEpisode 按集号选 episodeId；找不到则就近/取末集兜底。
func pickEpisode(eps []Episode, want int) string {
	if len(eps) == 0 {
		return ""
	}
	if want <= 0 {
		want = 1
	}
	for _, e := range eps {
		if parseLeadingInt(e.EpisodeNumber) == want {
			return e.EpisodeID.String()
		}
	}
	last := parseLeadingInt(eps[len(eps)-1].EpisodeNumber)
	if want >= last {
		return eps[len(eps)-1].EpisodeID.String()
	}
	best, bestDiff := eps[0], absInt(parseLeadingInt(eps[0].EpisodeNumber)-want)
	for _, e := range eps[1:] {
		if d := absInt(parseLeadingInt(e.EpisodeNumber) - want); d < bestDiff {
			best, bestDiff = e, d
		}
	}
	return best.EpisodeID.String()
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

type resolved struct {
	episodeID string
	name      string
}

// resolveEpisodeID 标题/季/集 -> episodeId（带缓存）。
func (c *LogVarClient) resolveEpisodeID(title string, season, episode int) (string, string, error) {
	key := "ep:" + normTitle(title) + ":" + strconv.Itoa(season) + ":" + strconv.Itoa(episode)
	if v, ok := c.cache.get(key); ok {
		r := v.(resolved)
		return r.episodeID, r.name, nil
	}
	animes, err := c.Search(title)
	if err != nil {
		return "", title, err
	}
	if len(animes) == 0 {
		return "", title, fmt.Errorf("no logvar results for %q", title)
	}
	bestIdx, bestScore := 0, -1e18
	for i := range animes {
		if s := c.score(&animes[i], title, season, episode); s > bestScore {
			bestScore, bestIdx = s, i
		}
	}
	chosen := animes[bestIdx]
	eps, err := c.Bangumi(strconv.FormatInt(chosen.AnimeID, 10))
	if err != nil {
		return "", chosen.AnimeTitle, err
	}
	epID := pickEpisode(eps, episode)
	if epID == "" {
		return "", chosen.AnimeTitle, fmt.Errorf("episode %d not found under %q", episode, chosen.AnimeTitle)
	}
	c.cache.set(key, resolved{epID, chosen.AnimeTitle}, c.store.Get().ResolveTTL)
	return epID, chosen.AnimeTitle, nil
}

// ResolveByTitle 完整流程：标题/季/集 -> 弹幕。
func (c *LogVarClient) ResolveByTitle(title string, season, episode int) ([]Comment, string, error) {
	id, name, err := c.resolveEpisodeID(title, season, episode)
	if err != nil {
		return nil, name, err
	}
	comments, err := c.Comment(id)
	if err != nil {
		return nil, name, err
	}
	return comments, name, nil
}
