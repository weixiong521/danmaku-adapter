---
AIGC:
    Label: "1"
    ContentProducer: 001191440300708461136T1XGW3
    ProduceID: 72bedbacb9db265375b893d98c8f8513_d6df12ecb7d311f1a59e525400248c00
    ReservedCode1: 7BWcLs44Y4jVh+AM1YDf2TLEMYDZ0c4+oX7XPiZNbxPgdKWI18S1E27+3EoM/S3BxLRVCvw2/U0CMKnAY+/cwWPL7b7QuJHF2c7EXrXG9i9wK8x758i9BytaM05yPOR/NHPSGGQf1O6DB+UMOJURlyKcVwGRitkGSEIZ9Gh94UUcJzi96sQ157e4CrA=
    ContentPropagator: 001191440300708461136T1XGW3
    PropagateID: 72bedbacb9db265375b893d98c8f8513_d6df12ecb7d311f1a59e525400248c00
    ReservedCode2: 7BWcLs44Y4jVh+AM1YDf2TLEMYDZ0c4+oX7XPiZNbxPgdKWI18S1E27+3EoM/S3BxLRVCvw2/U0CMKnAY+/cwWPL7b7QuJHF2c7EXrXG9i9wK8x758i9BytaM05yPOR/NHPSGGQf1O6DB+UMOJURlyKcVwGRitkGSEIZ9Gh94UUcJzi96sQ157e4CrA=
---



# LogVar → Getapp 弹幕适配服务（Docker 版）

把 **LogVar 弹幕 API** 对接到 **苹果CMS V10（MacCMS V10）+ 资源站（巨量/自建）+ Getapp（Go 独立版）** 的原生 APP，让 APP 播放器显示弹幕。

纯 Go 标准库实现，**零第三方依赖**，编译后是单个静态二进制；本版本额外提供 **Docker 一键部署** 与 **资源站在线热更新** 能力。

---

## 一、特性一览

| 特性 | 说明 |
|---|---|
| Docker 部署 | 多阶段构建，运行镜像基于 `alpine`，体积小、非 root 运行、自带健康检查 |
| 配置热重载 | 修改 `config.json` 后 **无需重启**，后台每 15s 自动检测并生效 |
| 资源站热更新 | 通过 `/admin` 接口 **实时增删资源站**，改完立即生效并持久化到磁盘 |
| 协议转换 | 对内调用 LogVar（弹弹play 规范），对外暴露 Getapp 协议 |
| 智能选源选集 | 按标题/季/集打分选源，可从资源站 m3u8 的 32 位 hash 反查剧名/季/集 |
| 苹果CMS 直连抓取 | `?ac=cms&id={视频ID}` / `?ac=cms&url={详情页URL}`：主动抓详情页全部分集 m3u8，再交给弹幕链路匹配 |
| 弹幕裁剪 | `max_danmu` 可按时间轴均匀抽样，避免手机端一次性渲染数万条 |

---

## 二、为什么需要这个服务

Getapp 与 LogVar 的弹幕协议不一致，中间需要一个“翻译层”：

| 方向 | 协议 |
|---|---|
| Getapp（APP）请求 | `GET /?ac=dm&url=<播放地址>&douban_id=<豆瓣ID>`，期望返回 `{"code":1,"name":"...","danum":N,"danmuku":[[时间,位置,颜色,"0",内容,IP,时间戳,字号],...]}` |
| LogVar 原生 | 弹弹play 规范：`/api/v2/search/anime`、`/api/v2/bangumi/{id}`、`/api/v2/comment/{id}`，弹幕字段是 `p="时间,类型,颜色,来源"` |

### 数据流

```
资源站 ──采集──> 苹果CMS V10 ──读库──> Getapp(Go) ──播放某一集──> 本适配服务
                                                          │
                     m3u8 里的内容 hash ──> 资源站分享页(剧名/季/集)
                                                          │
                     剧名/季/集 ──> LogVar(腾讯/爱奇艺/B站…聚合) ──> 弹幕
                                                          │
                                                转换为 Getapp 格式返回 APP

无 APP 场景（可选链路）：
本服务 ──?ac=cms&id={视频ID}──> 苹果CMS 详情页/播放页 ──> 全部集 m3u8 ──> 同上弹幕链路
```

---

## 三、快速开始（Docker，推荐）

### 3.1 目录准备

```bash
mkdir -p getapp-danmu/data && cd getapp-danmu
# 把本目录下的所有文件拷进来（Dockerfile / docker-compose.yml / *.go / docs 等）
```

### 3.2 生成配置

```bash
cp config.example.json data/config.json
```

按需修改 `data/config.json`（资源站、LogVar 地址、管理口令等，见第五节）。

### 3.3 启动

```bash
docker compose up -d --build
docker compose logs -f getapp-danmu
```

首次启动若 `data/config.json` 不存在，容器入口脚本会自动生成一份默认配置。

### 3.4 验证

```bash
curl http://127.0.0.1:12381/health          # 返回 ok
curl "http://127.0.0.1:12381/?ac=dm"        # 返回 {"code":1,"name":"","danum":0,"danmuku":[]}
```

### 3.5 不用 compose 也行

```bash
docker build -t getapp-danmu:1.0.0 .
docker run -d --name getapp-danmu \
  -p 12381:12381 \
  -e TZ=Asia/Shanghai \
  -e ADMIN_ENABLED=true -e ADMIN_TOKEN=change-me-please \
  -v "$PWD/data:/app/data" \
  --restart unless-stopped \
  getapp-danmu:1.0.0
```

### 3.6 交叉编译到 arm64（树莓派 / 部分云 ARM 机型）

```bash
docker buildx build --platform linux/arm64 -t getapp-danmu:1.0.0-arm64 --load .
```

### 3.7 不用 Docker：源码编译 / 直接用二进制

```bash
# 需要 Go 1.21+
CGO_ENABLED=0 go build -o getapp-danmu .
./getapp-danmu
```

也可直接上传随源码附带的 `getapp-danmu_linux_amd64` / `_linux_arm64`（`chmod +755` 后运行）。

---

## 四、配置说明

配置优先级：**环境变量 > `config.json` > 内置默认值**。

> ⚠️ 由于环境变量优先级最高，若你在 compose 里用环境变量设置了某个字段，
> 那么 `config.json` 与 `/admin` 接口对该字段的修改都会被环境变量覆盖。
> 建议：**资源站、选源、LogVar 地址** 只放在 `config.json` / 走 `/admin` 接口维护。

### 4.1 配置文件字段

| 参数名（config.json） | 类型 | 默认值 | 环境变量 | 说明 |
|---|---|---|---|---|
| `listen` | string | `:12381` | `LISTEN` / `PORT` | 监听地址 |
| `logvar_base` | string | `https://1.501710491.xyz/weixiong` | `LOGVAR_BASE` | LogVar 根地址（不含 `/api`） |
| `logvar_token` | string | `""` | `LOGVAR_TOKEN` | LogVar 若设置了访问令牌则填写 |
| `resource_hosts` | []string | `["https://jimaoys95.com","https://jimaoys94.com"]` | `RESOURCE_HOSTS` / `JULIANG_SHARE_HOSTS` | **资源站主机池**（分享页 + CDN），支持热更新 |
| `http_timeout_ms` | int | `45000` | `HTTP_TIMEOUT_MS` | 单次上游请求超时（毫秒） |
| `max_danmu` | int | `0` | `MAX_DANMU` | 单集弹幕上限，0 不限；超限按时间轴均匀抽样（建议 `8000`） |
| `font_size` | string | `24px` | `FONT_SIZE` | 弹幕字号字段 |
| `default_color` | string | `#FFFFFF` | `DEFAULT_COLOR` | 颜色解析失败时的兜底颜色 |
| `source_priority` | []string | `tencent,iqiyi,bilibili,youku,mango,migu,renren,kan360,douban,vod` | `SOURCE_PRIORITY` | 选源偏好（命中越靠前加分越高） |
| `resolve_ttl_sec` | int | `600` | `RESOLVE_TTL_SEC` | “剧名/季/集→episodeId” 解析缓存（秒） |
| `admin_enabled` | bool | `false` | `ADMIN_ENABLED` | 是否开启 `/admin` 管理接口 |
| `admin_token` | string | `""` | `ADMIN_TOKEN` | `/admin` 访问令牌（请求头 `X-Admin-Token`） |
| `watch_interval_sec` | int | `15` | `WATCH_INTERVAL_SEC` | 配置文件轮询间隔（秒） |
| `cms_base_url` | string | `""` | `CMS_BASE_URL` | 苹果CMS 站点根地址（不带 `/index.php`），供 `?ac=cms&id=..` 使用；内置默认空，`config.example.json` 中预填了示例站点。留空时只能用 `url` 传完整详情页地址 |
| `cms_concurrency` | int | `4` | `CMS_CONCURRENCY` | `?ac=cms` 逐集抓取播放页的并发数 |
| `cms_timeout_ms` | int | `30000` | `CMS_TIMEOUT_MS` | `?ac=cms` 单次上游请求超时（毫秒）；单次请求整体预算另有兜底（×4，下限 30s、上限 3min） |
| `cms_all_episodes` | bool | `true` | `CMS_ALL_EPISODES` | `?ac=cms` 默认是否抓取全部分集；`false` 时仅抓首集 |
| `cms_max_episodes` | int | `0` | `CMS_MAX_EPISODES` | `?ac=cms` 单次最多抓取集数，`0` 不限 |

### 4.2 仅环境变量（无对应文件字段）

| 环境变量 | 说明 |
|---|---|
| `CONFIG_PATH` | 配置文件路径，默认 `config.json`；Docker 中默认 `/app/data/config.json` |
| `TZ` | 时区，如 `Asia/Shanghai` |

---

## 五、资源站：添加 / 替换 / 实时更新

本服务的“资源站”即 **播放源主机池**（`resource_hosts`）。它有两个用途：

1. **识别播放地址**：当 APP 传来的播放地址命中资源站主机时，走“hash → 分享页 → 剧名/季/集”链路；
2. **解析分享页**：按顺序请求 `https://<host>/s/<32位hash>`，解析内嵌 `#share-boot` 得到剧名与当前集。

### 5.1 方式一：改配置文件（自动热重载）

编辑 `data/config.json` 的 `resource_hosts`，保存后 15 秒内自动生效，**无需重启**：

```json
{
  "resource_hosts": [
    "https://jimaoys95.com",
    "https://jimaoys94.com",
    "https://jimaoys83.com",
    "jimaoys82.com"
  ]
}
```

> 主机名可不带 `https://`，程序会自动补全并统一为 `https://主机名`；自动去重。

### 5.2 方式二：/admin 接口（立即生效，推荐）

先在配置中开启管理接口（`admin_enabled: true` 且设置 `admin_token`），然后：

| 方法 | 路径 | 作用 |
|---|---|---|
| GET | `/admin/sources` | 查看当前资源站 / 选源 / LogVar |
| POST | `/admin/sources` | **新增**资源站（追加） |
| PUT | `/admin/sources` | **整体替换**资源站 |
| DELETE | `/admin/sources` | **删除**资源站 |
| POST | `/admin/priority` | 修改选源优先级 |
| POST | `/admin/logvar` | 修改 LogVar 地址 / 令牌 |
| POST | `/admin/reload` | 手动触发配置重载 |

示例：

```bash
TOKEN=change-me-please
BASE=http://127.0.0.1:12381

# 查看
curl -H "X-Admin-Token: $TOKEN" $BASE/admin/sources

# 新增一个资源站（实时生效 + 落盘）
curl -X POST -H "X-Admin-Token: $TOKEN" -H "Content-Type: application/json" \
  -d '{"resource_hosts":["jimaoys83.com","https://jimaoys82.com/"]}' \
  $BASE/admin/sources

# 删除
curl -X DELETE -H "X-Admin-Token: $TOKEN" -H "Content-Type: application/json" \
  -d '{"resource_hosts":["https://jimaoys82.com"]}' \
  $BASE/admin/sources
# DELETE 不带 body 时也可： curl -X DELETE ".../admin/sources?host=jimaoys82.com"

# 整体替换
curl -X PUT -H "X-Admin-Token: $TOKEN" -H "Content-Type: application/json" \
  -d '{"resource_hosts":["jimaoys95.com","jimaoys94.com"]}' \
  $BASE/admin/sources

# 调整选源优先级
curl -X POST -H "X-Admin-Token: $TOKEN" -H "Content-Type: application/json" \
  -d '{"source_priority":["tencent","iqiyi","bilibili"]}' \
  $BASE/admin/priority

# 更换 LogVar 上游
curl -X POST -H "X-Admin-Token: $TOKEN" -H "Content-Type: application/json" \
  -d '{"logvar_base":"https://your-logvar.example.com/xxx","logvar_token":""}' \
  $BASE/admin/logvar
```

> 所有修改都会**立即生效**并写回 `config.json`，容器重启后依然保留。
> 管理接口默认关闭；开启后请务必设置足够复杂的 `ADMIN_TOKEN`，并尽量只在内网/反代鉴权后暴露。

---

## 六、第三步：Getapp（Go 独立版）对接

进入 Getapp 后台 → **系统 → 第三方弹幕接口**：

1. **第三方弹幕接口地址** 填本服务地址（程序会自动在后面拼 `&douban_id=..&url=..`，**不要**自己加这些参数）：
   - 直连：`http://你的服务器IP:12381/?ac=dm`
   - 域名（推荐）：`https://dm.你的域名/?ac=dm`
2. **自定义域名（`third_danmu_sort`）**：资源站多为自有 CDN 域名（如 `jimaoysXX.com`），默认不会被传给弹幕接口。需要把资源站域名加进来，例如：
   ```
   jimaoys95.com,jimaoys94.com,jimaoys83.com,jimaoys82.com,jimaoys81.com,jimaoys80.com
   ```
   编号会随时间轮换；若发现新采集的剧没弹幕，把新的域名补进来即可。
3. **弹幕地址类型（`system_third_danmu_url_type`）保持 0（传播放地址）**。不要设为 1——设为 1 会把 `url` 置空，资源站内容将无法匹配。
4. 保存后，在 APP 里打开一集影片即可看到弹幕。

> iOS 的 ATS 要求弹幕接口为 https，建议用 Nginx/宝塔做反代 + SSL：
> `https://dm.你的域名/` → `http://127.0.0.1:12381/`。

---

## 七、验证与自测

```bash
BASE=http://127.0.0.1:12381

# 1) 官方平台地址（应返回大量弹幕）
curl "$BASE/?ac=dm&url=$(python3 -c 'import urllib.parse;print(urllib.parse.quote("https://v.qq.com/x/cover/rjae621myqca41h/i0032qxbi2v.html"))')"

# 2) 资源站 m3u8（核心：自动反查剧名/季/集）
curl "$BASE/?ac=dm&url=$(python3 -c 'import urllib.parse;print(urllib.parse.quote("https://jimaoys94.com/public/playback/c365c8cf36c195bd4aa367677ad84ae8/smart.m3u8"))')"

# 3) 无参数（应返回 danum=0）
curl "$BASE/?ac=dm"

# 4) 苹果CMS 直连抓取（无 APP 场景：只抓 m3u8）
curl "$BASE/?ac=cms&id=2421&danmu=0"
```

返回形如：

```json
{"code":1,"name":"庆余年 第一季(2019)【国产剧】from tencent","danum":40457,
 "danmuku":[["0.00","right","#FF1964","0","弹幕内容","127.0.0.1","1790185274","24px"]]}
```

字段含义见 [`docs/danmuku-format.md`](docs/danmuku-format.md)。

---

## 八、苹果CMS 直连抓取（`?ac=cms`）

在**没有 APP、拿不到播放地址**的场景下，直接按“视频 ID 或详情页 URL”去苹果CMS 站点抓全部分集 m3u8，再走与 `?ac=dm` 完全一致的弹幕链路。

### 8.1 请求

```bash
BASE=http://127.0.0.1:12381

# 只抓 m3u8，不做弹幕匹配
curl "$BASE/?ac=cms&id=2421&danmu=0"

# 抓全量集数并匹配弹幕
curl "$BASE/?ac=cms&id=2421&all=1"

# 仅第 1 播放源的第 3 集
curl "$BASE/?ac=cms&id=2421&sid=1&nid=3"

# 用详情页 URL（等价路径 /cms 亦可用）
curl --get --data-urlencode "url=https://www.501710491.xyz/index.php/vod/detail/id/2421.html" \
     --data "ac=cms" "$BASE/"
```

| 参数 | 必填 | 说明 |
|---|---|---|
| `id` | 二选一 | 苹果CMS 视频 ID |
| `url` | 二选一 | 详情页 URL（传播放页 URL 亦可，会自动提取 ID）；别名 `detail_url` / `cms_url` |
| `sid` / `nid` | 否 | 限定播放源 / 集号（指定 `nid` 后自动只抓该集） |
| `all` | 否 | `1/0`，覆盖配置 `cms_all_episodes` |
| `match_all` | 否 | `1/0`，是否合并返回所有集的弹幕 |
| `danmu` | 否 | `0` 只抓 m3u8；默认 `1` |

### 8.2 返回

在原有 `code` / `name` / `danum` / `danmuku` 之外，附带 `cms` 明细：`id` / `title` / `base` / `detail_url` / `sources[]`（`sid` / `from` / `name` / `episodes`）/ `episodes[]` / `ok_count` / `total` / `danmu_name`。
每集含 `sid` / `nid` / `name` / `ok` / `encrypt` / `m3u8` / `page_url` / `danum`，失败时带 `error`，另有可选 `sid_name` / `from` / `url_next` / `danmu_name`。示例与字段说明见 [`docs/cms-direct-crawl.md`](docs/cms-direct-crawl.md)。

### 8.3 抓取链路

1. `GET /index.php/vod/detail/id/{id}.html` → 解析全部 `/vod/play/id/{id}/sid/{sid}/nid/{nid}.html`，按 `sid` 归并分集；剧名优先取 `<h1>`，退化再从 `<title>` 剥离站点后缀。
2. 并发抓取各集播放页 → 正则定位 `var player_aaaa = {` → **花括号配平**提取完整对象 → JSON 解析。
3. `encrypt == 0` 时 `url` 即真实 m3u8；`encrypt == 1` 交给 `cmsDecryptHook` 预留钩子（现覆盖 URL-encode / base64 两种伪加密）。
4. m3u8 交给现有链路：资源站 hash 反查剧名/季/集 → LogVar 匹配；**若该链路无弹幕，回退用“详情页剧名 + 集号”再匹配一次**。

### 8.4 注意

- 站点若挂在 Cloudflare 后，请求已补齐 `Referer` 与浏览器常规头；仍返回 5xx 时先确认站点本身可访问。
- 只支持**服务端渲染**的分集列表（苹果CMS V10 标准路由）；纯 JS 动态渲染的播放列表抓不到。
- 报错遵循本项目惯例：参数错误 → `code=0`；抓取链路中单集失败 → 记入该集 `ok=false` / `error`，不影响其它集。

### 8.5 验证与冒烟

```bash
# 单元测试（不需要外网）
go test ./...

# 真实站点端到端冒烟：详情页 → 播放页 → m3u8 → 弹幕匹配 → HTTP 接口
CMS_SMOKE=1 go test -tags smoke -run TestCMSSmoke -v .
```

冒烟用例在 `smoke_test.go`（构建标签 `smoke`），默认不参与普通构建；未设置 `CMS_SMOKE=1` 时自动跳过，可用 `CMS_SMOKE_BASE` / `CMS_SMOKE_ID` 覆盖站点与视频 ID。

---

## 九、工作原理与已知限制

**匹配逻辑**

- 传入腾讯/爱奇艺/B站等 **官方平台地址** → 直接让 LogVar 按地址取弹幕，最准。
- 传入 **资源站 m3u8**（含 32 位内容 hash）→ 抓取分享页 `/s/<hash>`，解析内嵌 `#share-boot` JSON，得到剧名与当前集，再让 LogVar 按剧名匹配聚合弹幕。
- 只有 **douban_id** → 先查豆瓣标题；电影可直接匹配，剧集因 Getapp 默认不带集数无法定位。

**限制**

1. 资源站 CDN 域名会轮换，需在 Getapp 的“自定义域名”与本服务 `resource_hosts` 中同步维护。
2. Getapp 默认只传“播放地址 + 豆瓣ID”，**不传集数**。剧集依赖 m3u8 中的 hash 定位集数；走“豆瓣ID + 空地址”模式（type=1）时剧集无法确定是哪一集，故不建议。
3. 弹幕丰富度取决于 LogVar 上游是否有该片；冷门片可能为空，属正常。
4. 单集弹幕可能数万条，建议 `max_danmu` 限制（如 8000）以减轻手机压力。
5. `listen` 修改后需重启容器才会重新绑定端口（端口无法热切换）。

---

## 十、目录结构

```
logvar-getapp-docker/
├── Dockerfile               # 多阶段构建
├── docker-compose.yml       # 一键部署 + 健康检查
├── docker-entrypoint.sh     # 首启生成默认配置
├── .dockerignore
├── config.example.json      # 配置示例
├── go.mod
├── main.go                  # 入口/路由/优雅退出
├── config.go                # 配置加载 + 热重载（ConfigStore）
├── sources.go               # 资源站运行时管理（SourceManager）
├── admin.go                 # /admin 管理接口
├── handler.go               # Getapp 弹幕入口 + 路由分发
├── handle_cms.go            # ?ac=cms 处理器（抓取编排 + 弹幕匹配）
├── cms.go                   # 苹果CMS 直连抓取（详情页/播放页解析、player_aaaa、解密钩子）
├── logvar.go                # LogVar 客户端：搜索/详情/弹幕 + 评分选源选集
├── juliang.go               # 资源站分享页解析（hash→剧名/季/集）+ 豆瓣解析
├── transform.go             # LogVar 弹幕 → Getapp danmuku 格式转换
├── common.go                # 标题/季/集/中文数字解析
├── http.go                  # HTTP 客户端（重试）+ TTL 缓存
├── main_test.go             # 单元测试
├── cms_test.go              # 苹果CMS 抓取相关单元测试
├── smoke_test.go            # 真实站点端到端冒烟（-tags smoke，默认不编译）
└── docs/
    ├── danmuku-format.md    # 弹幕库字段说明（参数名/类型/说明/示例）
    ├── core-files.md        # 核心文件说明
    ├── cms-direct-crawl.md  # 苹果CMS 直连抓取（?ac=cms）详解
    └── tutorial.md          # 完整使用教程
```

---

## 十一、文档索引

| 文档 | 内容 |
|---|---|
| [`docs/danmuku-format.md`](docs/danmuku-format.md) | 弹幕接口返回结构、`danmuku` 8 字段参数名/类型/说明/示例 |
| [`docs/core-files.md`](docs/core-files.md) | 每个核心文件的职责、关键函数、调用关系 |
| [`docs/cms-direct-crawl.md`](docs/cms-direct-crawl.md) | 苹果CMS 直连抓取：接口、配置、抓取链路、排障 |
| [`docs/tutorial.md`](docs/tutorial.md) | 从零部署 → 对接 Getapp → 加资源站 → 排障的完整教程 |

---

## 十二、Docker 化改造分析

### 12.1 原项目形态与痛点

原始 `logvar-getapp` 是**单个 Go 二进制 + systemd 服务**的形态：

| 痛点 | 影响 |
|---|---|
| 配置写死在 `config.json`，改完必须重启 | 资源站域名一换就得重启服务，弹幕请求中断 |
| 资源站主机池硬编码 | 新增资源站要改配置 + 重启，无法在线调整 |
| 无环境变量支持 | 容器化后无法用 `docker run -e` 注入差异配置 |
| 无健康检查 | 编排系统无法判断服务是否真的可用 |
| 无优雅退出 | `stop` 时正在处理的请求被硬杀 |
| 依赖宿主机的 Go 环境 | 换机器需重新配编译环境 |

### 12.2 改造点对照

| 维度 | 改造前 | 改造后 |
|---|---|---|
| 交付形态 | 二进制 + systemd unit | 多阶段构建镜像 + `docker compose` |
| 配置来源 | 仅 `config.json` | **环境变量 > `config.json` > 内置默认值** 三级合并 |
| 配置生效 | 重启进程 | `ConfigStore` + 后台 Watcher，**改文件 15s 内热生效** |
| 资源站管理 | 改文件重启 | `SourceManager` + `/admin/sources`，**在线增删改、立即生效、自动落盘** |
| 并发安全 | 全局变量 | `sync.RWMutex` 快照 + 原子替换（clone→改副本→Set），无锁读取 |
| 运行用户 | root | 镜像内 `app` 非 root 用户 |
| 健康探针 | 无 | `HEALTHCHECK` 轮询 `/health`（compose 中亦声明） |
| 时区 | 依赖宿主 | 镜像内置 `tzdata` + `TZ=Asia/Shanghai` |
| 证书 | 依赖宿主 | 镜像内置 `ca-certificates`，HTTPS 上游开箱可用 |
| 数据持久化 | 直接写工作目录 | `VOLUME /app/data`，配置挂在卷上 |
| 优雅退出 | 无 | `signal.NotifyContext` + `srv.Shutdown`，5s 排空 |
| 首启体验 | 需手工建配置 | 入口脚本自动由 `config.example.json` 生成 |
| 镜像体积 | — | 多阶段构建 + `-ldflags "-s -w"` + `CGO_ENABLED=0`，运行层仅 alpine + ~10MB 二进制 |

### 12.3 关键设计取舍

1. **为什么不把资源站管理做成数据库/Redis？**
   配置体量极小（几十个域名），用 `config.json` 作为唯一真相源 + 内存快照，既零依赖又能直接备份迁移。

2. **为什么用“改文件 + 轮询 mtime”而不是 `fsnotify`？**
   容器场景下挂载卷的 inotify 事件在部分宿主机/Windows 共享目录上不可靠；轮询 mtime 跨平台稳定，15s 延迟对域名变更场景完全够用，且不引入第三方依赖。

3. **为什么所有失败都返回 HTTP 200 + 空弹幕？**
   APP 端对非 200 会走异常分支或重试风暴。返回标准空结构能让 APP 静默降级为“无弹幕”，体验更稳。

4. **为什么环境变量优先级最高？**
   便于容器编排统一注入；同时通过文档明确提示：想用 `/admin` 在线改的字段，就不要在 compose 里设同名环境变量，避免“改了不生效”的困惑。

5. **为什么 `admin` 默认关闭？**
   管理接口可改上游地址，属敏感能力。默认关闭 + 常量时间比较令牌 + 文档建议内网/反代鉴权，是安全默认值。

### 12.4 遗留风险

| 风险 | 说明 |
|---|---|
| 端口无法热切换 | `listen` 变更需重启容器（HTTP Server 重绑成本高，收益低） |
| 配置文件写入冲突 | 若同时有外部程序写 `config.json` 与 `/admin` 写回，可能互相覆盖；建议只用一种方式管理 |
| 容器重建时未挂载 `data/` | 配置丢失。已在 compose 中默认挂载，请勿删除 |
| 上游依赖外部可用性 | LogVar 与资源站均为第三方，服务本身无法保证其长期可用 |
*（内容由AI生成，仅供参考）*
*（内容由AI生成，仅供参考）*
