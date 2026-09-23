---
AIGC:
    Label: "1"
    ContentProducer: 001191440300708461136T1XGW3
    ProduceID: 72bedbacb9db265375b893d98c8f8513_5dd4e4f8b78c11f199d2525400393706
    ReservedCode1: kC1ytD6OJLr+63qBGXuD06wLB9tbi7FQROpNJY/7ZdI/0g3N73miDUXR02hQBNBrgeYdu+/baWCM8uSs7EKkb6GU1RuWaL0RutM2h6LlRUQ2pIT01j4KlpTNySGMPUgipuzCfdN0pTp+VEWC3vN4WXKU6PndcJhzUFNnpcnSMLzTYbSUjQy1XWX8otM=
    ContentPropagator: 001191440300708461136T1XGW3
    PropagateID: 72bedbacb9db265375b893d98c8f8513_5dd4e4f8b78c11f199d2525400393706
    ReservedCode2: kC1ytD6OJLr+63qBGXuD06wLB9tbi7FQROpNJY/7ZdI/0g3N73miDUXR02hQBNBrgeYdu+/baWCM8uSs7EKkb6GU1RuWaL0RutM2h6LlRUQ2pIT01j4KlpTNySGMPUgipuzCfdN0pTp+VEWC3vN4WXKU6PndcJhzUFNnpcnSMLzTYbSUjQy1XWX8otM=
---

# 核心文件说明

本文逐个说明工程内每个文件的职责、关键类型与函数，以及整体调用关系。

---

## 一、总览

```
                        ┌──────────────────────────────┐
        HTTP 请求 ────> │  main.go  路由 + 优雅关闭     │
                        └───────────────┬──────────────┘
                                        │
                    ┌───────────────────┴────────────────────┐
                    ▼                                        ▼
        ┌───────────────────────┐                ┌────────────────────────┐
        │ handler.go 弹幕入口    │                │ admin.go 管理接口       │
        │  /  /dm  /danmu       │                │  /admin/*              │
        └──────────┬────────────┘                └───────────┬────────────┘
                   │                                          │
     ┌─────────────┼───────────────┐                          ▼
     ▼             ▼               ▼              ┌────────────────────────┐
┌─────────┐  ┌───────────┐  ┌────────────┐         │ sources.go SourceManager│
│logvar.go│  │juliang.go │  │transform.go│         │  资源站运行时增删改查    │
│LogVar   │  │资源站/豆瓣 │  │格式转换     │         └───────────┬────────────┘
└────┬────┘  └─────┬─────┘  └────────────┘                     │
     │             │                                          ▼
     └─────────────┴──────────────┬──────────────────> ┌──────────────────┐
                                  ▼                     │ config.go         │
                          ┌───────────────┐             │ ConfigStore 热重载 │
                          │ http.go       │             └──────────────────┘
                          │ 重试 + TTL 缓存 │
                          └───────────────┘
                                  │
                          ┌───────────────┐
                          │ common.go     │
                          │ 标题/季/集解析 │
                          └───────────────┘
```

---

## 二、文件逐个说明

### 1. `main.go` — 程序入口

**职责**：装配依赖、注册路由、启动后台热重载、优雅关闭。

| 函数/类型 | 说明 |
|---|---|
| `main()` | 创建 `ConfigStore` → `Server` → `http.ServeMux`，注册 `/`、`/dm`、`/danmu`、`/health`、`/admin/*`；`signal.NotifyContext` 监听 `SIGINT/SIGTERM`，收到后 5s 内优雅关闭 |
| `configPath()` | 返回配置路径：环境变量 `CONFIG_PATH` 优先，否则 `config.json` |
| `nowUnix()` | 返回当前 Unix 秒，供 `transform.go` 填充时间戳 |

**路由表**

| 路径 | 说明 |
|---|---|
| `/`、`/dm`、`/danmu` | 弹幕入口，等价 |
| `/health` | 健康检查，返回纯文本 `ok` |
| `/admin/sources` | 资源站增删改查 |
| `/admin/priority` | 选源优先级 |
| `/admin/logvar` | LogVar 上游地址/令牌 |
| `/admin/reload` | 手动触发配置重载 |

---

### 2. `config.go` — 配置加载与热重载（**核心**）

**职责**：三级配置合并、线程安全快照、文件变更监听、回写持久化。

| 类型/函数 | 说明 |
|---|---|
| `Config` | 运行期配置结构体（含 `ResourceHosts`、`AdminEnabled`、`WatchInterval` 等） |
| `fileConfig` | `config.json` 的映射结构，字段名与 JSON key 一致；未设置的指针字段为 `nil` 以便区分“未配置” |
| `defaultConfig()` | 内置默认值 |
| `(*Config).clone()` | 深拷贝切片字段，用作热更新的稳定快照 |
| `loadConfigFrom(path)` | 读文件 → 叠加环境变量 → 规范化（补 `https://`、去尾斜杠、去重） |
| `ConfigStore` | 持锁持有“当前生效配置”，提供 `Get/Set/Reload/Persist/Watch` |
| `(*ConfigStore).Get()` | 返回当前快照（`RWMutex` 读锁，高频调用无阻塞竞争） |
| `(*ConfigStore).Reload()` | 依据磁盘 + 环境变量重建配置并原子替换 |
| `(*ConfigStore).Persist()` | 把当前配置写回 `config.json`（`/admin` 接口调用） |
| `(*ConfigStore).Watch(ctx)` | 按 `watch_interval_sec` 轮询文件 mtime，变化即热重载 |
| `toFileConfig()` | 运行配置反向序列化为 JSON 结构 |
| `displayPath()` | 配置路径为空时显示“(默认值)” |

**关键设计**：所有读配置的地方都走 `store.Get()`，因此任何一次热更新都会立刻被所有请求看到，且不存在读到“半更新”状态的可能。

---

### 3. `sources.go` — 资源站运行时管理（**核心**）

**职责**：资源站的增、删、改、查与规范化；落实“可实时替换添加”这一需求。

| 类型/函数 | 说明 |
|---|---|
| `SourceManager` | 仅持有 `*ConfigStore`，**无内部状态**，天然并发安全 |
| `NewSourceManager(store)` | 构造 |
| `(*SourceManager).Snapshot()` | 供 `GET /admin/sources` 输出（令牌打码） |
| `normalizeHost(h)` | `jimaoys95.com` → `https://jimaoys95.com`；去尾斜杠、转小写、保留 `host` |
| `normalizeHosts(list)` | 规范化 + 去重，保持输入顺序 |
| `AddResourceHosts(list)` | **新增**（clone → 追加 → 原子替换 → 落盘），返回 `added` 与全量 |
| `RemoveResourceHosts(list)` | **删除**，返回 `removed` 与全量 |
| `SetResourceHosts(list)` | **整体替换** |
| `SetSourcePriority(list)` | 替换选源优先级 |
| `SetLogVar(base, token)` | 替换 LogVar 上游 |
| `HasResourceHost(rawURL)` | 判断地址是否命中已知资源站主机 |
| `maskToken(t)` | 令牌中间打码，用于接口回显 |

**关键设计**：每次修改都是 “`clone()` → 改副本 → `Set()` 原子替换 → `Persist()` 落盘”，
既保证并发读安全，又保证重启不丢配置。

---

### 4. `admin.go` — 管理接口

**职责**：`/admin` 路由分发与令牌鉴权。

| 函数 | 说明 |
|---|---|
| `(*Server).registerAdmin(mux)` | 注册 `/admin/` |
| `(*Server).adminAuthorized(r)` | 校验 `admin_enabled`；未设 token 放行；否则用 **常量时间比较** 校验 `X-Admin-Token` 或 `?token=` |
| `(*Server).handleAdmin(w, r)` | 按路径分发到四个 handler |
| `readAdminBody(w, r)` | 解析 JSON（限 1MB）；无 body 时兼容 `?host=a&host=b` |
| `adminSources` | `GET` 查看 / `POST` 新增 / `PUT` 替换 / `DELETE` 删除 |
| `adminPriority` | `POST` 修改选源优先级 |
| `adminLogvar` | `POST` 修改 LogVar 地址/令牌 |
| `adminReload` | `POST` 手动重载配置 |

---

### 5. `handler.go` — 弹幕入口与路由分发（**核心**）

**职责**：解析 Getapp 请求，决定走哪条匹配链路，组装并返回响应。

| 函数/类型 | 说明 |
|---|---|
| `isPlatformURL(u)` | 判断是否为官方平台地址（腾讯/爱奇艺/芒果/B站/优酷/咪咕/搜狐/乐视/抖音/西瓜/云视听 等） |
| `firstNonEmpty(...)` | 取第一个非空字符串 |
| `Server` | 聚合 `store` / `lv`(LogVar) / `jl`(资源站解析) / `sm`(资源站管理) |
| `(*Server).writeJSON(w, v)` | 统一输出 JSON；设置 CORS `*` 与 `Cache-Control: no-store` |
| `(*Server).handleDanmu(w, r)` | 主逻辑：`OPTIONS` 预检 → 取参 → 三条匹配分支 → 组装响应 |

**三条匹配分支（优先级从高到低）**

1. **官方平台地址** → `LogVar.CommentByURL(url)` 直接按地址取弹幕（最准）。
2. **资源站 hash** → `ExtractHash(url, resource_hosts)` 判定 → `Juliang.ResolveByHash` 得剧名/季/集 → `LogVar.ResolveByTitle` 取弹幕。
3. **豆瓣 ID 兜底** → `Juliang.ResolveDouban` 得标题；电影直接匹配；剧集需有集数参数。

**响应包**：`{code, name, danum, danmuku}`；任何失败均返回 `danum=0` 的空弹幕而非错误码。

---

### 6. `logvar.go` — LogVar 客户端与选源算法

**职责**：调用 LogVar 三个接口，用打分算法选出正确的条目与集。

| 类型/函数 | 说明 |
|---|---|
| `Anime` / `Episode` / `Comment` | 上游数据结构；`Episode.EpisodeID` 用 `json.Number` 避免 int32 溢出 |
| `LogVarClient` | 持有 `ConfigStore` + `http.Client` + TTL 缓存 |
| `apiPath(p)` | 拼接 `base[/token] + p`，令牌可变 |
| `Search(keyword)` | `/api/v2/search/anime` |
| `Bangumi(animeID)` | `/api/v2/bangumi/{id}` → 集列表 |
| `Comment(episodeID)` | `/api/v2/comment/{id}` → 弹幕列表 |
| `CommentByURL(videoURL)` | `/api/v2/comment?url=` → 按播放地址取弹幕 |
| `(*LogVarClient).score(...)` | **选源打分**：标题完全匹配 +100 / 相似 +60 / 不符 -60；季号匹配 +40 否则 -20；`source` 在 `source_priority` 中按位次加分；`vod` 源 -30（m3u8 通常取不到弹幕）；集数够 +5 |
| `pickEpisode(eps, want)` | 先精确匹配集号，超出取末集，否则取最近集 |
| `resolveEpisodeID(title, season, ep)` | 搜索 → 打分选条目 → 选集 → 缓存 |
| `ResolveByTitle(...)` | 标题/季/集 → 弹幕 + 命中剧名 |

---

### 7. `juliang.go` — 资源站分享页解析 + 豆瓣兜底

**职责**：把 m3u8 里的内容 hash 还原成“剧名 + 第几季 + 第几集”。

| 类型/函数 | 说明 |
|---|---|
| `reJLHash` | 匹配 32 位十六进制内容标识 |
| `reShareBoot` | 匹配 `<script id="share-boot">…</script>` 内嵌 JSON |
| `ShareBoot` / `NavUnit` | 分享页结构：`contentTitle` 剧名、`unit` 当前单元、`navigation[]` 单元列表 |
| `ExtractHash(rawURL, resourceHosts)` | 提取 hash；同时返回 `looksJL` 表示是否属于“可解析资源站”（命中配置主机 或 含 `jimaoys` / `/playback/` / `/s/`） |
| `NewJuliangResolver(store)` | 构造 |
| `ResolveByHash(hash)` | **依次遍历 `resource_hosts`** 请求 `/s/<hash>`，解析 `#share-boot`，用 `unit` 在 `navigation` 中定位当前季/集；带 TTL 缓存 |
| `DoubanDetail` / `ResolveDouban(id)` | 豆瓣兜底：返回标题与类型（`movie` / `tv`） |

**关键设计**：多资源站顺序尝试，任一成功即返回，天然容错资源站域名轮换。

---

### 8. `transform.go` — 格式转换（**核心**）

**职责**：LogVar 弹幕 → Getapp `danmuku` 数组。

| 函数 | 说明 |
|---|---|
| `buildDanmuku(comments, cfg, now)` | 主转换：逐条解析 `p`，输出 8 元素数组；`max_danmu` 生效时按时间轴均匀抽样 |
| `sampleComments(in, n)` | 按 `i*len/n` 等间隔抽 `n` 条，保留整片时间跨度 |
| `decColorToHex(s, fallback)` | 十进制整数 → `#RRGGBB`；非法或越界用 `fallback` |
| `modeToPosition(mode)` | `1→right`、`2→top`、`3→bottom`、`4→left`，其余默认 `right` |

> 抽样策略：超过 `max_danmu` 时按时间顺序等间隔抽取，保留完整时间跨度、避免“只有前几分钟有弹幕”。

字段映射详见 [`danmuku-format.md`](danmuku-format.md) 第 2.4 节。

---

### 9. `common.go` — 解析工具

| 函数 | 说明 |
|---|---|
| `parseAnimeTitle(raw)` | 从 `"庆余年 第二季(2019)【国产剧】from tencent"` 拆出 `(基础标题, 季号, 年份)` 三个返回值 |
| `titleEqual(a, b)` | 标题严格相等（归一化后比较） |
| `titleSimilar(a, b)` | 归一化后互相包含即判相似 |
| `normTitle(s)` | 归一化标题（转小写、去空白与常见标点），用于缓存 key 与比较 |
| `parseLeadingInt(s)` | 从 `"12"`、`"第12集"` 中取第一个整数，失败返回 0 |
| `parseCNNumber(s)` | 中文数字转整数（如 `十二` → `12`，支持 `1..99`），失败返回 0 |

---

### 10. `http.go` — HTTP 客户端

| 函数/类型 | 说明 |
|---|---|
| `httpGetWithRetry(ctx, client, url, headers, retries, timeout)` | 单次请求带超时；失败指数退避重试；返回 `body, status, err` |
| `ttlCache` | 极简 TTL 缓存，`get/set`；用于“剧名→episodeId”与“hash→剧名”两个高频映射 |

---

### 11. `main_test.go` — 单元测试

覆盖标题解析、季号提取、hash 提取、颜色转换、抽样等纯函数，可 `go test ./...` 运行。

---

## 三、依赖注入关系

```
ConfigStore ──┬──> SourceManager（读写配置：资源站/优先级/上游）
              ├──> LogVarClient（读配置：上游地址/超时/选源偏好/缓存 TTL）
              └──> JuliangResolver（读配置：资源站主机/超时/缓存 TTL）
                      │
                      └──> Server（聚合三者 + 路由） ──> main.go
```

所有组件共享**同一个** `ConfigStore` 实例，这是“改一次配置全局立即生效”的实现基础。

---

## 四、线程安全说明

| 组件 | 保护方式 |
|---|---|
| `ConfigStore.cur` | `sync.RWMutex`，`Get` 只读锁、`Set` 写锁 |
| `ConfigStore.lastMod` | 独立 `sync.Mutex` |
| `ttlCache` | 内部 `sync.RWMutex` |
| `SourceManager` | 无状态，靠 `ConfigStore` 保证 |
| HTTP handler | Go `net/http` 每请求独立 goroutine，无共享可变状态 |

---

## 五、扩展点

| 想加什么 | 改哪里 |
|---|---|
| 新的资源站来源类型（非分享页） | `juliang.go` 增加 Resolver 分支，在 `handler.go` 加判定 |
| 新的弹幕上游（除 LogVar 外） | 仿 `logvar.go` 写新 Client，`handler.go` 里按配置切换 |
| 更多输出协议（如自定义 APP） | 仿 `transform.go` 写新构造器，`handler.go` 按参数选择 |
| 更多管理能力（如限流开关） | 在 `config.go` 加字段，`admin.go` 加路由 |
*（内容由AI生成，仅供参考）*
