---
AIGC:
    Label: "1"
    ContentProducer: 001191440300708461136T1XGW3
    ProduceID: 72bedbacb9db265375b893d98c8f8513_d802d7c6b7d311f1a59e525400248c00
    ReservedCode1: ZUWeTS5KtOhq20rcm0Pw8fp5CF3R7V95uImGNr/9iov6EK1JC5bQBfNjl/KIhwGAtlHqdyTjeFkkPEpg8z9i8yk7i4HaIlm0S4aDksls22npveGpBNTLGZwjW85Ab991LLil8gFtjgwKPYsOvr61Kf6HAZUMhzl0WRxtN+em7Ji00yN8b7NKevrgOBQ=
    ContentPropagator: 001191440300708461136T1XGW3
    PropagateID: 72bedbacb9db265375b893d98c8f8513_d802d7c6b7d311f1a59e525400248c00
    ReservedCode2: ZUWeTS5KtOhq20rcm0Pw8fp5CF3R7V95uImGNr/9iov6EK1JC5bQBfNjl/KIhwGAtlHqdyTjeFkkPEpg8z9i8yk7i4HaIlm0S4aDksls22npveGpBNTLGZwjW85Ab991LLil8gFtjgwKPYsOvr61Kf6HAZUMhzl0WRxtN+em7Ji00yN8b7NKevrgOBQ=
---

# 苹果CMS 直连抓取 + 弹幕匹配（`?ac=cms`）

面向「只有视频详情页 / 视频 ID，但拿不到播放地址」的场景：直接向**苹果CMS V10 站点**抓取该视频的全部 m3u8，再把 m3u8 交给现有 LogVar 弹幕链路匹配弹幕。

---

## 一、适用场景

| 场景 | 传统 `?ac=dm` 是否可用 | `?ac=cms` 是否可用 |
|---|---|---|
| Getapp 播放时传来 m3u8 播放地址 | ✅ | ✅ |
| 只有豆瓣 ID | ⚠️ 剧集无法定位集数 | ✅（需要站点 ID/URL） |
| 只有苹果CMS 详情页 URL 或视频 ID | ❌ | ✅ |
| 批量导出某片全部集数的 m3u8 | ❌ | ✅ |

---

## 二、接口

```
GET /?ac=cms&id={视频ID}
GET /?ac=cms&url={详情页URL}
```

> 等价路径：`/cms?id=...`（`/`、`/dm`、`/danmu`、`/cms` 共用同一处理器，按 `ac` 参数或路径分发）。

### 2.1 请求参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `id` | 二选一 | 苹果CMS 视频 ID，如 `2421` |
| `url` | 二选一 | 视频详情页 URL；传**播放页 URL** 亦可（会自动提取 ID）。别名：`detail_url`、`cms_url` |
| `sid` | 否 | 仅处理指定播放源（如 `1`） |
| `nid` | 否 | 仅处理指定集（如 `3`）；指定后自动关闭“全量抓取” |
| `all` | 否 | `1/0`，覆盖配置 `cms_all_episodes`：是否抓取全部集 |
| `match_all` | 否 | `1/0`，是否把所有集的弹幕合并返回；默认仅返回“选中集”的弹幕 |
| `danmu` | 否 | `0` 表示只抓 m3u8、不做弹幕匹配；默认 `1` |

### 2.2 返回结构

沿用现有协议（`code` / `name` / `danum` / `danmuku`），并额外附带 `cms` 明细：

```json
{
  "code": 1,
  "name": "灵猪降妖",
  "danum": 171,
  "danmuku": [["0.00", "right", "#FFFFFF", "0", "弹幕内容", "127.0.0.1", "1790185274", "24px"]],
  "cms": {
    "id": "2421",
    "title": "灵猪降妖",
    "base": "https://www.501710491.xyz",
    "detail_url": "https://www.501710491.xyz/index.php/vod/detail/id/2421.html",
    "total": 1,
    "ok_count": 1,
    "danmu_name": "灵猪降妖(2026)【华语电影】from tencent",
    "sources": [
      { "sid": 1, "from": "jlm3u8", "name": "巨量资源", "episodes": 1 }
    ],
    "episodes": [
      {
        "sid": 1,
        "nid": 1,
        "name": "第 1 集",
        "ok": true,
        "encrypt": 0,
        "page_url": "https://www.501710491.xyz/index.php/vod/play/id/2421/sid/1/nid/1.html",
        "m3u8": "https://jimaoys90.com/public/playback/6cec284de4970e079a2e4267c4b11e79/smart.m3u8",
        "danum": 171,
        "danmu_name": "可爱的中国-新西游研学旅行记之了不起的青岛(2022)【综艺】from 360"
      }
    ]
  }
}
```

> 上例为真实站点抓取结果（站点采集数据会随时变化，ID/集数以实际为准）。

字段说明：

| 字段 | 说明 |
|---|---|
| `cms.id` / `cms.title` / `cms.base` / `cms.detail_url` | 视频 ID、站点剧名、站点根地址、实际请求的详情页地址 |
| `cms.sources[]` | 详情页解析出的播放源：`sid`（源序号）、`from`（播放器标识）、`name`（源显示名）、`episodes`（集数） |
| `cms.episodes[]` | 每集抓取结果：`sid` / `nid` / `name` / `ok` / `encrypt` / `m3u8` / `page_url` / `danum`；失败时带 `error`，另有可选字段 `sid_name` / `from` / `url_next` / `danmu_name` |
| `cms.ok_count` / `cms.total` | 成功抓到的集数 / 本次请求集数 |
| `cms.danmu_name`（顶层同名字段） | 本次实际匹配到弹幕的剧名，由资源站 hash 反查得到，**可能与站点剧名不同** |

**错误约定**：沿用本项目惯例——参数错误（缺 `id`/`url`、无法解析 ID）返回 `{"code":0,"msg":"..."}`；抓取链路内的单集失败**不**影响整体，记在该集的 `ok=false` / `error` 字段中。

---

## 三、配置项

配置优先级仍为 **环境变量 > `config.json` > 默认值**。

| config.json | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `cms_base_url` | `CMS_BASE_URL` | `""`（`config.example.json` 中预填了示例站点） | 苹果CMS 站点根地址（不带 `/index.php`）；留空则只能用 `url` 入参且必须为完整 URL |
| `cms_concurrency` | `CMS_CONCURRENCY` | `4` | 逐集请求播放页的并发数 |
| `cms_timeout_ms` | `CMS_TIMEOUT_MS` | `30000` | 单次上游请求（详情页/播放页）超时（毫秒）；整体预算由 `cmsOverallTimeout` 兜底（×4，下限 30s、上限 3min） |
| `cms_all_episodes` | `CMS_ALL_EPISODES` | `true` | 默认是否抓取全部集；`false` 时仅抓首集 |
| `cms_max_episodes` | `CMS_MAX_EPISODES` | `0` | 单次最多抓取集数，`0` 表示不限（用于控制大剧集的耗时） |

---

## 四、抓取链路

```
① 解析入参
   id / 详情页 URL / 播放页 URL  →  base + id
        ↓
② GET {base}/index.php/vod/detail/id/{id}.html
   正则提取全部 /vod/play/id/{id}/sid/{sid}/nid/{nid}.html
   → 按 sid 归并成分集列表；剧名优先取 <h1>，退化再从 <title> 剥离站点后缀
        ↓
③ 逐集（并发受 cms_concurrency 限制）
   GET {base}/index.php/vod/play/id/{id}/sid/{sid}/nid/{nid}.html
   正则定位 var player_aaaa = {  →  花括号配平提取完整 JSON  →  JSON 解析
        ├─ encrypt == 0  →  url 字段即真实 m3u8
        └─ encrypt == 1  →  交给 cmsDecryptHook 预留钩子（依次尝试 URL-decode / base64，均失败返回明确错误）
        ↓
④ m3u8 → 现有弹幕链路（与 ?ac=dm 完全一致）
   4.1 m3u8 命中资源站主机 → 取 32 位 hash → 分享页 /s/<hash> → 剧名/季/集 → LogVar 匹配
   4.2 上述链路无弹幕时 → 回退「详情页剧名 + 集号」再匹配一次
```

### 关键实现点

- **花括号配平提取**：`player_aaaa` 内含嵌套对象，且 `url` 里可能出现 `{}`，因此不用非贪婪正则截断，而是从 `{` 起按字符串感知的括号配平取到配对 `}`（`extractBalancedObject`）。
- **请求头**：苹果CMS 站点常挂在 Cloudflare 之后，除 `Referer` 外补齐 `Accept` / `Accept-Language` / `Sec-Fetch-*` 等浏览器常规头，降低被判定为爬虫返回 5xx 的概率。
- **重试**：复用 `httpGetWithRetry`，状态码非 200 时按策略重试，最终错误信息带状态码（如 `upstream HTTP error status 503`），便于排障。
- **encrypt=1 钩子**：`cmsDecryptHook` 为预留解密入口，目前覆盖 URL-decode 与 base64（标准 / URL-safe / 无填充）这类常见“伪加密”，且要求解出的串确实含 `.m3u8` 才采纳；真实站点若使用 MacPlayer 专属算法，只需在该函数内补实现，无需改抓取流程。
- **详情页缓存**：`ResolveDetail` 的结果进 TTL 缓存，短时间内重复请求同一视频不会反复打站点。
- **整体超时**：单次 `?ac=cms` 的总预算为 `cms_timeout_ms × 4`（下限 30s、上限 3min），避免大剧集抓取无限拖长。

---

## 五、示例

```bash
BASE=http://127.0.0.1:12381

# 1) 只抓 m3u8，不匹配弹幕
curl "$BASE/?ac=cms&id=2421&danmu=0"

# 2) 抓全量集数并匹配弹幕
curl "$BASE/?ac=cms&id=2421&all=1"

# 3) 只处理第 1 播放源的第 3 集
curl "$BASE/?ac=cms&id=2421&sid=1&nid=3"

# 4) 传详情页 URL
curl --get --data-urlencode "url=https://www.501710491.xyz/index.php/vod/detail/id/2421.html" \
     --data "ac=cms" "$BASE/"

# 5) 合并返回所有集的弹幕
curl "$BASE/?ac=cms&id=2421&all=1&match_all=1"
```

---

## 六、排障

| 现象 | 排查方向 |
|---|---|
| `无法从入参解析出视频 ID` | `url` 必须是详情页/播放页形式（含 `/id/{数字}` 或 `?id={数字}`） |
| `详情页请求失败：upstream HTTP error status 503` | 站点被 Cloudflare 拦截或站点临时不可用；先用浏览器/curl 验证该详情页可访问，并确认 `cms_base_url` 正确 |
| `详情页未解析出任何分集` | 站点非苹果CMS V10 标准路由，或播放列表由 JS 动态渲染（本项目只解析服务端渲染的分集链接） |
| 某集 `encrypt=1` 且 `error` 提示无法解密 | 该站点启用了 MacPlayer 加密播放地址，需在 `cmsDecryptHook` 中补对应算法 |
| 有 m3u8 但弹幕为 0 | ① 该片 LogVar 上游没有；② 资源站域名未加入 `resource_hosts`；③ `max_danmu` 之外的选源问题 |

---

## 七、测试

- **单元测试**：`cms_test.go` 覆盖基础地址规范化、ID 提取、分集解析、`player_aaaa` 解析（含嵌套对象与字符串花括号）、括号配平、`encrypt` 解密钩子、剧名清洗。
- **真实站点冒烟**：`smoke_test.go`（构建标签 `smoke`，默认不参与普通构建）跑通「详情页 → 播放页 → m3u8 → 弹幕匹配 → HTTP 接口」全链路：

  ```bash
  CMS_SMOKE=1 go test -tags smoke -run TestCMSSmoke -v .
  ```

  未设置 `CMS_SMOKE=1` 时自动跳过，避免误触发外部请求；站点与视频 ID 可用 `CMS_SMOKE_BASE` / `CMS_SMOKE_ID` 覆盖。

---

## 八、与 `?ac=dm` 的关系

两者共用同一套弹幕匹配与格式转换代码（`logvar.go` / `juliang.go` / `transform.go`）。区别仅在**如何拿到 m3u8**：

- `?ac=dm` 由 APP 传入播放地址；
- `?ac=cms` 由本服务主动去苹果CMS 站点抓取。

因此 `?ac=cms` 天然具备“无 APP 场景”的能力：可当作**独立接口**用于批量导出 m3u8 或批量抓取弹幕。
*（内容由AI生成，仅供参考）*
