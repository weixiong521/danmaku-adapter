---
AIGC:
    Label: "1"
    ContentProducer: 001191440300708461136T1XGW3
    ProduceID: 72bedbacb9db265375b893d98c8f8513_ecc52143b79b11f199d2525400393706
    ReservedCode1: alYR8OShjmTaeYtuIMvluD3JdN4q751Cdpo+eL7YTLKgd/visK0T3XdYmdqp+s6/GAaVWttPSzmxxuo/ljJKb9VhCJwpR3gEiVl2OdG877x9uVzaYXv3lVIQm+DZ3PK+qTb9p0/jrsBILV8GlghsWr4E8LvYhcuCd9uFw3ln/GqA+PB+aIIR0p/smE8=
    ContentPropagator: 001191440300708461136T1XGW3
    PropagateID: 72bedbacb9db265375b893d98c8f8513_ecc52143b79b11f199d2525400393706
    ReservedCode2: alYR8OShjmTaeYtuIMvluD3JdN4q751Cdpo+eL7YTLKgd/visK0T3XdYmdqp+s6/GAaVWttPSzmxxuo/ljJKb9VhCJwpR3gEiVl2OdG877x9uVzaYXv3lVIQm+DZ3PK+qTb9p0/jrsBILV8GlghsWr4E8LvYhcuCd9uFw3ln/GqA+PB+aIIR0p/smE8=
---

# 弹幕与剧集/剧名错配问题排查报告

> 排查对象：`logvar-getapp-docker`（LogVar → Getapp 弹幕适配服务）
> 排查范围：从接收播放地址 / 豆瓣 ID 到搜索、选中剧集、返回弹幕的完整匹配链路
> 结论性质：静态代码分析（本机无 Go 编译与运行环境，未做线上抓包验证；每条结论均标注对应代码位置，可对照复核）

---

## 一、结论速览

按"最可能导致错配"的优先级排序：

| # | 根因 | 代码位置 | 典型表现 |
|---|---|---|---|
| 1 | **选源打分无接受阈值**：只要 LogVar 返回任何结果就必然选中一个（初始分 -1e18），哪怕全部候选都被判为"不同剧" | `logvar.go` `resolveEpisodeID()` | `name` 返回完全不相干的剧名 |
| 2 | **集数选不到时强制兜底**："就近取集"甚至"取末集" | `logvar.go` `pickEpisode()` | 剧名对、但弹幕是别的集（如第 40 集请求拿到第 24 集弹幕） |
| 3 | **want 侧标题未归一化 + 年份被丢弃**：`parseAnimeTitle` 只作用于候选，`year` 直接丢弃 | `logvar.go` `score()` | 同名不同年份/不同版本（翻拍）互相串台 |
| 4 | **`titleSimilar` 双向包含匹配过宽**、无长度约束、无"续集/外传/剧场版"黑名单 | `common.go` `titleSimilar()` | 《斗罗大陆》命中《斗罗大陆2绝世唐门》/剧场版 |
| 5 | **资源站分享页解析失败时静默降级为第 1 季第 1 集**，且该错误结果被写入 10 分钟缓存 | `juliang.go` `ResolveByHash()` | 剧名对、永远只放第 1 集弹幕 |
| 6 | **播放地址里明明带集数线索却被完全忽略**，集数 100% 依赖分享页 `navigation` 匹配 | `juliang.go` `ExtractHash()` / `ResolveByHash()` | 资源站集号与 LogVar 集号整体错位 |
| 7 | **豆瓣兜底分支把剧集当电影**：`type=movie` 时固定 `ResolveByTitle(title, 1, 1)` | `handler.go` `handleDanmu()` | 每一集都返回第 1 集弹幕 |
| 8 | **多资源站"第一个非空即 break"**，不校验与 APP 实际播放地址是否同站 | `juliang.go` `ResolveByHash()` | 剧名后缀/版本与所看电影不一致 |

---

## 二、完整匹配链路

```
Getapp APP
  │  GET /?ac=dm&url=<播放地址>&douban_id=<豆瓣ID>     （默认不带集数！）
  ▼
[1] handler.go: handleDanmu()          入口分流
  ├─ isPlatformURL(url) 命中官方平台  → lv.CommentByURL(url)        ← 最准，无兜底
  ├─ ExtractHash(url,rhosts)  looksJL → jl.ResolveByHash(hash)      ← 主链路
  │                                     → lv.ResolveByTitle(title, season, episode)
  └─ douban_id 非空                   → jl.ResolveDouban(id)
                                        → movie: ResolveByTitle(title,1,1)
                                        → tv+ep : ResolveByTitle(title,1,ep)
  ▼
[2] juliang.go: ExtractHash()          播放地址 → 32 位 hash（取"第一个" hex 串）
  ▼
[3] juliang.go: ResolveByHash()        hash → /s/<hash> → #share-boot
                                        → ContentTitle（剧名）、navigation+unit（季/集）
  ▼
[4] logvar.go: resolveEpisodeID()      关键词搜索 + 打分选剧
     ├─ Search(title)                  ← title 未清洗，直接当关键词
     ├─ score()                        候选 vs want 打分（want 未归一化、year 丢弃）
     ├─ Bangumi(animeId)               → episodes
     └─ pickEpisode(eps, want)         ← 找不到就"就近/末集"兜底
  ▼
[5] logvar.go: Comment(episodeId)      取弹幕原文
  ▼
[6] transform.go: buildDanmuku()       转 Getapp 格式 + max_danmu 均匀抽样
  ▼
APP 播放器
```

---

## 三、逐环节问题清单

### 环节 1：入口参数（`handler.go` `handleDanmu`）

| 问题 | 说明 |
|---|---|
| 集数完全不可靠 | Getapp 只传 `url` + `douban_id`，代码里 `ep/episode` 仅是"非 Getapp 默认字段"的兜底，实际几乎永远拿不到 → 集数只能靠分享页解析，解析失败即错集 |
| 分支互斥无回退 | `switch` 中资源站分享页解析失败后 `break`（只跳出 switch），不会回退尝试 `douban_id`，也没有"两条链路结果交叉校验" |
| 无一致性校验 | 拿到 `url` 与 `douban_id` 时，二者对应的剧名从未做一致性比对（同为《XX》的剧集/电影/翻拍版极易串台） |
| 平台分支过于宽松 | `isPlatformURL` 基于 `.qq.com`/`.bilibili.com` 等子串，命中后 `name` 直接回填整条 URL，且无任何校验与兜底 |

### 环节 2：地址 → hash（`juliang.go` `ExtractHash`）

| 问题 | 说明 |
|---|---|
| 取"第一个" 32 位 hex | `reJLHash = [0-9a-f]{32}`，`FindString` 取 URL 中**第一个**匹配。若 URL 含 CDN 签名/鉴权 token（同为 32 位 hex），会取到**非内容 ID** → 请求 `/s/<token>` 拿到 404 或别的页 |
| 站点识别硬编码 | `looksJL` 依赖 `jimaoys`、`/playback/`、`/s/` 三个特征；用户后加的**非巨量资源站**若路径不含这些特征、且其 m3u8 走的是另一个 CDN 域名（不在 `resource_hosts` 内），则 `looksJL=false` → 直接掉到豆瓣分支 → 剧集无集数 → 空返回或错配 |
| 集数线索被丢弃 | 大量资源站 m3u8 形如 `/public/playback/<hash>/1000/index.m3u8`、`/hls/<hash>/1/23/index.m3u8`，路径里的 `1000`/`23` 很可能就是集号，现在**完全没有解析** |
| 返回 hash 与 host 无绑定 | 只返回 hash，后续遍历所有资源站去试，无法保证"用播放该片的那个站"解析 |

### 环节 3：分享页解析（`juliang.go` `ResolveByHash`）

| 问题 | 说明 |
|---|---|
| **UnitID 匹配失败 → 默认 (1,1)** | `season, episode := 1, 1`，仅当 `u.UnitID == boot.Unit` 才覆盖。分享页为**剧集总览页**、`unit` 字段为空/格式变化时，静默退化为第 1 季第 1 集，**且不返回错误** |
| 错误结果被缓存 | `r.cache.set(key, jlResolved{...}, cfg.ResolveTTL)` 无条件执行（默认 600s），(1,1) 这种错误结果会被钉死 10 分钟 |
| 缓存 key 粒度不足 | 仅 `jl:<hash>`；若同一 hash 为**剧级 ID**（所有集共用），首次解析出的集号会被所有集复用 |
| 多站"先到先得" | 遍历 `ResourceHosts`，第一个 `ContentTitle` 非空即 break，不判断"哪个站才是 APP 实际播放的站"，也不比较多个站的解析一致性 |
| 标题未清洗 | `boot.ContentTitle` 原样作为搜索关键词，可能带 `【】`、`-第X季`、`全46集`、`免费在线观看` 等后缀（`parseAnimeTitle` 只在**候选侧**生效） |
| 超时预算过大 | `overall` = `HTTPTimeout × 2 × len(hosts) + 30s`（默认 2 站 = 210s），单次分享页请求最长 45s，容易让 Getapp 侧先超时重试，造成"同一集反复请求、结果漂移" |
| 内容校验缺失 | 只用 `ContentTitle != ""` 判断成功，未校验 `shareKey`/`contentId` 是否与请求的 hash 对应，误命中别的页面也会被接受 |

### 环节 4：搜索与选剧打分（`logvar.go` `resolveEpisodeID` / `score`）

| 问题 | 说明 |
|---|---|
| **无最低分阈值** | `bestIdx, bestScore := 0, -1e18`，循环后必选一个候选。若 LogVar 返回的候选全部是别的内容（每项都吃 `-60`），仍会选中 → 直接返回毫不相干的弹幕 |
| **want 未归一化** | `score()` 中 `parseAnimeTitle()` 只解析**候选** `a.AnimeTitle`；`wantTitle` 是原始串。带"第2季/(2024)/全46集"的 want 使 `titleEqual(base, wantTitle)`（+100 档）**几乎永远不成立**，全部退化为 `titleSimilar`（+60 档），打分基准被削弱 |
| **年份被丢弃** | `base, season, _ := parseAnimeTitle(a.AnimeTitle)` —— `year` 解析出来了却不用。同名不同年（翻拍剧，如《射雕英雄传》1983/2017/2024）无法区分，选错版本即"弹幕剧情对不上" |
| **包含匹配过宽** | `titleSimilar` 为双向 `strings.Contains`，无长度占比约束。`want="斗罗大陆"` → 命中 `"斗罗大陆2绝世唐门"`、`"斗罗大陆剧场版"`；2~3 字短标题（如《寻秦记》）误命中率更高 |
| 未使用别名 | `Anime.Aliases` 字段已解析但打分完全没用；LogVar 同一部剧常以多个 `animeTitle` 出现（带副标题、原名/译名），别名是最有效的消歧手段 |
| 续集类标记无惩罚 | `外传/番外/OVA/剧场版/特别篇/花絮/合集/预告/幕后` 等词未纳入负向特征 |
| 季号兜底无区分 | 分享页给不出季时 wantSeason=1，于是"第 1 季"和其他剧一样吃 `season==wantSeason` 的 +40，反过来放大错误 |
| 源偏好权重偏高 | `s += 25 - i*1.5`（首位 +25），与"季匹配 ±40/-20"同量级，可能出现"季错了但源偏好高"被选中 |
| `EpisodeCount` 项无效 | `a.EpisodeCount >= wantEp` 的 ±5/-10，在 `wantEp` 多为 1 时无区分度 |
| 搜索关键词单一 | 只用 `Search(title)` 搜一次，没有"基础剧名 / 剧名+第N季 / 剧名+年份"的多轮召回与合并 |

### 环节 5：集数定位（`logvar.go` `pickEpisode`）

| 问题 | 说明 |
|---|---|
| **就近兜底** | 精确集号找不到时，`want >= last` 取末集，否则取"集号差最小"的集 → **静默返回错误集的弹幕** |
| 空集号陷阱 | `parseLeadingInt("")`、`"SP1"`、`"OVA"` 等一律返回 0；若某源 `episodeNumber` 全为空，则 `want >= last(0)` 恒真 → **永远返回最后一集** |
| 免费送第 1 集 | `want <= 0` 直接改写为 1 → 集数未知时必然返回第 1 集弹幕 |
| 未处理集号偏移/跨季连续 | LogVar 的 bangumi 若按"跨季连续编号"（第 2 季为 25~48），want=2 会取不到并兜底到第 25 集；资源站含"预告/花絮"导致集号整体偏移时，整季错位 |
| 未校验总数 | 不比较 `len(eps)` 与分享页给出的集数/资源站集数，集数不匹配时也不会告警 |

### 环节 6：兜底与输出（`handler.go` / `juliang.go` / `transform.go`）

| 问题 | 说明 |
|---|---|
| 豆瓣电影分支硬编码季集 | `d.Type == "movie"` → `ResolveByTitle(d.Title, 1, 1)`；若豆瓣 `type` 缺失或把剧集标成 movie，则**每集都返回第 1 集弹幕** |
| 豆瓣剧集分支忽略真实季 | tv 分支固定 `season=1`，只用 `extraEp` |
| 弹幕无内容校验 | `Comment(episodeId)` 返回后不校验弹幕总量/时间轴是否与片长匹配，错配无法被下游发现 |
| LogVar 错误码被忽略 | `getJSON` 不检查 `errorCode`/`success`，上游返回错误结构时静默返回空弹幕，排查困难 |
| 诊断信息不足 | 全链路只在 `handleDanmu` 打了一行"资源站 -> 剧名=%q 第%d季 第%d集"，中间候选与得分、分享页原始 `unit`、URL 集号线索都没打印 |
| 反查难度 | 返回的 `name` 是 `chosen.AnimeTitle`（可用来判断"剧错"还是"集错"），但 APP 端不一定展示，建议同时输出到日志 |

---

## 四、修复方案

### P0 — 立即消除"明知不对还返回"的行为

**P0-1 集数严格化：找不到就返回空，不再就近/末集兜底**

`logvar.go` `pickEpisode`：

```go
// 返回 error 而非兜底，让上层决定"返回空弹幕"
func pickEpisode(eps []Episode, want int) (string, error) {
    if len(eps) == 0 {
        return "", fmt.Errorf("empty episode list")
    }
    if want <= 0 {
        return "", fmt.Errorf("unknown episode number")
    }
    for _, e := range eps {
        if parseLeadingInt(e.EpisodeNumber) == want {
            return e.EpisodeID.String(), nil
        }
    }
    // 仅在"该剧只有一集(剧场版/电影)"时才允许唯一值兜底
    if len(eps) == 1 {
        return eps[0].EpisodeID.String(), nil
    }
    return "", fmt.Errorf("episode %d not found under %d episodes", want, len(eps))
}
```

对应地，`resolveEpisodeID` 在 `pickEpisode` 报错时返回错误；`handler` 捕获后记录日志并返回 `danum=0`（比返回别集的弹幕好得多）。

**P0-2 打分增加"强匹配门槛 + 最低分阈值"**

`logvar.go`：

```go
// 归一化后的强标题匹配：完全相等，或长短串包含且短串占比 ≥ 60%
func strongTitleMatch(candidate, want string) bool {
    a, b := normTitle(candidate), normTitle(want)
    if a == "" || b == "" {
        return false
    }
    if a == b {
        return true
    }
    la, lb := len([]rune(a)), len([]rune(b))
    if strings.Contains(a, b) && lb*10 >= la*6 {
        return true
    }
    if strings.Contains(b, a) && la*10 >= lb*6 {
        return true
    }
    return false
}

// 续集类标记：候选带、want 不带 → 直接排除
var sequelMarks = []string{"第二季", "第三季", "2", "3", "Ⅱ", "Ⅲ", "外传", "番外",
    "剧场版", "特别篇", "花絮", "合集", "预告", "幕后", "OVA", "精编版", "cut"}

func hasSequelMark(s string) bool { /* 归一化后 contains 任一标记 */ }
```

`resolveEpisodeID` 里改成"先硬过滤、再加权打分、最后设阈值"：

```go
bestIdx, bestScore := -1, -1e18
const minAccept = 60.0 // 最低接受分：至少"强标题匹配(+100) + 季匹配(+40)"级别
for i := range animes {
    base, season, year := parseAnimeTitle(animes[i].AnimeTitle)
    if !strongTitleMatch(base, wantBase) && !aliasMatch(animes[i].Aliases, wantBase) {
        continue                                  // 硬门槛：标题不像就淘汰
    }
    if hasSequelMark(base) && !hasSequelMark(wantBase) {
        continue                                  // 不要把续集/剧场版当正片
    }
    if s := c.score(&animes[i], wantBase, wantSeason, wantYear); s > bestScore {
        bestScore, bestIdx = s, i
    }
}
if bestIdx < 0 || bestScore < minAccept {
    return "", "", fmt.Errorf("no confident match for %q (best=%.0f)", wantBase, bestScore)
}
```

### P1 — 提升匹配精度

1. **want 侧先归一化**：进入 `resolveEpisodeID` 前对传入 title 执行 `parseAnimeTitle`，得到 `wantBase / seasonFromTitle / wantYear`。
   季号取值优先级：分享页 `navigation` 明确给出的 > 标题解析出的 > 1。
2. **年份参与打分**（当前完全没用）：`year == wantYear`（都非 0）→ `+25`；都非 0 且不等 → `-40`；want 无年份则不参与。
3. **用上 `Aliases`**：任一别名归一化后等于 `wantBase` → `+80`，并视为强匹配通过项。
4. **收敛相似度**：把 `titleSimilar` 的双向 `Contains` 换成 P0-2 的 `strongTitleMatch`，仅在"结果很少"时才回退到宽松模式。
5. **负向特征**：`外传/番外/剧场版/OVA/特别篇/花絮/合集` 等（见上）无条件降权/淘汰。
6. **多轮召回**：先搜 `wantBase`；若结果为空或全部不合格，再搜 `wantBase + " 第N季"`（N 为渴望季号），合并候选后统一打分。

### P2 — 集数与来源可靠性

1. **解析 URL 自带集号**：新增 `ExtractEpisodeHint(rawURL) int`，在 `/playback/<hash>/<n>/index.m3u8`、`/hls/<hash>/<a>/<b>/index.m3u8`、`第23集` 等形态下提取集号线索，作为 `episode` 的候选（与分享页解析结果**取交集**，不一致时以"分享页 navigation"为准并记 warning）。
   > ⚠ 不同资源站的集号编码规则不同（有的 `1000` 表示第 1 集、有的直接是集号），建议先把线上真实 m3u8 URL 形态打日志采样，再定正则，避免又引入新的错配。
2. **分享页解析失败不再静默**：`Navigation` 为空或 `unit` 未命中时，返回 `error`（或至少打 warning），而不是 `(1,1)`。
3. **错误结果不入缓存**：只有"标题非空 **且** 集号来自 navigation 命中"时才 `cache.set`；否则不缓存，避免错误被固化 10 分钟。
4. **缓存 key 加集号**：`jl:<hash>:<unitId>`，防剧级 hash 跨集复用。
5. **优先同站解析**：若播放地址命中 `resource_hosts` 中某站，优先用该站解析分享页；多站候选时对比 `contentTitle` 归一化是否一致，不一致则记 warning 并优先取同站结果。
6. **压缩超时**：分享页单次请求 3~5s，整链路总预算 ≤ 8~10s（当前 45s×2×站数+30s 太长）；豆瓣单次 ≤ 5s。超时直接返回空弹幕，避免 APP 端超时重试导致结果漂移。

### P3 — 可观测性（便于持续定位）

1. 增加 `DEBUG_MATCH=1` 开关，在日志中输出：请求 `url`/`douban_id`、提取到的 hash、分享页原始 `unit` 与 `navigation` 摘要、候选剧列表及各自得分、最终 `chosen.AnimeTitle/animeId/source/season/episode`。
2. `getJSON` 校验 `errorCode`/`success`，失败时把 `errorMessage` 写入日志。
3. `handler` 的 `switch` 改为"可回退链"：资源站解析失败 → 尝试豆瓣 → 都失败返回空；并保留每步失败原因到日志。
4. 在响应里保留 `name` 透出实际选中的 `animeTitle`（便于线上快速判断"剧错"还是"集错"）。

---

## 五、线上快速判定（3 步定位）

| 观察到的现象 | 结论 | 对应修复 |
|---|---|---|
| 返回 `name` 与当前剧**一致**，但弹幕内容/时间轴对不上 | 集数选错（就近兜底、集号偏移、URL 集号未用） | P0-1、P2-1~6 |
| 返回 `name` 是**另一部剧**（或同名不同年版本） | 选剧打分问题（无阈值、want 未归一化、年份/别名未用、包含匹配过宽） | P0-2、P1-1~6 |
| 返回 `name` 带奇怪后缀（`-第2季`、`全46集`） | 分享页 `ContentTitle` 未清洗即当关键词 | P1-1 |
| 剧名对、但**任何一集**都是同一批弹幕 | 分享页 `unit` 未命中 → 固定 (1,1) 且被缓存 | P2-2、P2-3、P2-4 |
| 每集都返回第 1 集弹幕，且走的是豆瓣链路 | `type=movie` 分支硬编码 `(1,1)` | 环节 6 第 1 条（豆瓣分支按 `type`/`ep` 精细化） |

---

## 六、实施顺序建议

1. 先落 **P0-1 + P0-2**（改动局部、收益最大，能立刻把"错弹幕"变成"空弹幕"）。
2. 再落 **P1**（匹配精度，需少量回归测试：准备 10~20 个已知"剧名/季/集"的样本做对照）。
3. **P2** 中 P2-2/3/4 与 P0 同期做；P2-1（URL 集号）需先采样线上 URL 形态再定规则。
4. **P3** 与以上并行，用于验证修复效果。

> 说明：本报告仅做定位与方案设计，未改动任何源码（本机无 Go/Docker 环境，改动无法编译验证）。如需我直接落地 P0 补丁，告知即可。
*（内容由AI生成，仅供参考）*
