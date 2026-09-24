---
AIGC:
    Label: "1"
    ContentProducer: 001191440300708461136T1XGW3
    ProduceID: 72bedbacb9db265375b893d98c8f8513_d9b0df9ab7d311f1a59e525400248c00
    ReservedCode1: 1kc1/1EC7QC1kiJQstryUhFOuBTjhCxn33SQg2DwDGRgYAFRqXEd0R2XJ4rsVNqUicOVIpeYvkctM/f/lRGwOLIQoG2Yc1bdWmeQiVE4Zf+ObDcL/9nThlsQtSO30NXT+Tyvy01a9kVgBgjBw7gdMIneqw/RWpXL+ebmEvgL6hlOq3G7UV0CdLyZeVI=
    ContentPropagator: 001191440300708461136T1XGW3
    PropagateID: 72bedbacb9db265375b893d98c8f8513_d9b0df9ab7d311f1a59e525400248c00
    ReservedCode2: 1kc1/1EC7QC1kiJQstryUhFOuBTjhCxn33SQg2DwDGRgYAFRqXEd0R2XJ4rsVNqUicOVIpeYvkctM/f/lRGwOLIQoG2Yc1bdWmeQiVE4Zf+ObDcL/9nThlsQtSO30NXT+Tyvy01a9kVgBgjBw7gdMIneqw/RWpXL+ebmEvgL6hlOq3G7UV0CdLyZeVI=
---



# 完整使用教程

从零开始：把 LogVar 弹幕接到 Getapp（苹果CMS V10）APP 上，并学会在线增删资源站。
按顺序照做即可，全程约 10 分钟。

---

## 目录

1. [前置条件](#一前置条件)
2. [第一步：部署服务](#二第一步部署服务docker)
3. [第二步：验证服务可用](#三第二步验证服务可用)
4. [第三步：对接 Getapp 后台](#四第三步对接-getapp-后台)
5. [第四步：APP 端验证](#五第四步app-端验证)
6. [第五步：添加 / 替换资源站](#六第五步添加--替换资源站)
7. [第六步：苹果CMS 直连抓取（可选）](#七第六步苹果cms-直连抓取可选)
8. [第七步：日常运维](#八第七步日常运维)
9. [排障 FAQ](#九排障-faq)
10. [进阶：反向代理与 HTTPS](#十进阶反向代理与-https)
11. [完整链路回顾](#十一完整链路回顾)

---

## 一、前置条件

| 项 | 要求 |
|---|---|
| 服务器 | Linux（amd64 / arm64 均可），1 核 1G 足够 |
| Docker | Docker 20.10+ 与 Docker Compose v2（`docker compose version` 能输出即满足） |
| 网络 | 服务器需能访问：LogVar 上游、资源站分享页、豆瓣（可选） |
| 端口 | 默认 `12381` 需可被 Getapp 所在机器访问 |
| 已有环境 | 一套可用的 Getapp（苹果CMS V10 + Getapp Go 独立版） |

> 没有 Docker？也可以直接编译或用随附的 `getapp-danmu_linux_amd64` 二进制，见 README 第 3.7 节。

---

## 二、第一步：部署服务（Docker）

### 2.1 上传工程

把整个 `logvar-getapp-docker` 目录上传到服务器，例如 `/opt/logvar-getapp-docker`。

### 2.2 准备配置

```bash
cd /opt/logvar-getapp-docker
mkdir -p data
cp config.example.json data/config.json
```

编辑 `data/config.json`，重点改这几项：

```json
{
  "resource_hosts": ["https://jimaoys95.com", "https://jimaoys94.com"],
  "logvar_base": "https://1.501710491.xyz/weixiong",
  "max_danmu": 8000,
  "admin_enabled": true,
  "admin_token": "换成你自己的复杂口令"
}
```

> 只有视频 ID / 详情页 URL 想直接抓 m3u8 时，还需要 `cms_base_url` 等字段，见本教程第七步。

### 2.3 启动

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f --tail=50 getapp-danmu
```

日志出现下面这句即成功：

```
LogVar-Getapp 弹幕适配服务启动：监听 :12381，资源站=2 个，管理接口=true
```

### 2.4 首次启动做了什么

1. 入口脚本检查 `/app/data/config.json`，不存在则从示例生成；
2. 以非 root 用户 `app` 启动主程序；
3. 程序读取配置并启动后台热重载协程；
4. 注册健康检查 `GET /health`。

---

## 三、第二步：验证服务可用

```bash
BASE=http://127.0.0.1:12381

# 1. 健康检查
curl $BASE/health
# -> ok

# 2. 空请求（应返回 code=1 / danum=0）
curl "$BASE/?ac=dm"
# -> {"code":1,"name":"","danum":0,"danmuku":[]}

# 3. 官方平台地址（B站/腾讯等，应返回大量弹幕）
curl -G "$BASE/" --data-urlencode "ac=dm" \
     --data-urlencode "url=https://v.qq.com/x/cover/rjae621myqca41h/i0032qxbi2v.html"
# -> {"code":1,"name":"庆余年 第一季(2019)【国产剧】from tencent","danum":40457,...}

# 4. 资源站 m3u8（验证 hash → 剧名/季/集 链路）
curl -G "$BASE/" --data-urlencode "ac=dm" \
     --data-urlencode "url=https://jimaoys94.com/public/playback/c365c8cf36c195bd4aa367677ad84ae8/smart.m3u8"
```

- 第 3 步有弹幕：说明**服务本体 + LogVar 上游**正常。
- 第 4 步有弹幕：说明**资源站解析链路**正常。
- 第 4 步为空但第 3 步正常：多半是 `resource_hosts` 里没有当前资源站域名，见第六步。

---

## 四、第三步：对接 Getapp 后台

登录 Getapp（苹果CMS V10）后台，进入 **系统 → 第三方弹幕接口**：

| 字段 | 填什么 | 说明 |
|---|---|---|
| 第三方弹幕接口地址 | `http://服务器IP:12381/?ac=dm` | 程序自动在其后拼 `&douban_id=..&url=..`，**不要**手写这些参数 |
| 自定义域名（`third_danmu_sort`） | `jimaoys95.com,jimaoys94.com,jimaoys83.com,jimaoys82.com` 等 | 资源站 CDN 域名白名单；不填则资源站地址不会被提交给弹幕接口 |
| 弹幕地址类型（`system_third_danmu_url_type`） | `0` | 0=传播放地址（**必须**）；1=传空地址，会导致资源站内容匹配不到 |
| 弹幕开关 | 开启 | — |

保存后清一次 Getapp 缓存（后台 → 系统 → 缓存管理）。

> **为什么必须配置“自定义域名”**：Getapp 默认只把“官方平台域名”的播放地址提交给第三方弹幕接口。资源站用的是自有 CDN 域名，不加白名单就会被过滤掉，服务收不到 `url`，自然没有弹幕。

---

## 五、第四步：APP 端验证

1. 用 Getapp APP 打开一部资源站采集的剧（如《庆余年》）；
2. 进入播放页，右上角菜单里确认弹幕开关已打开；
3. 若几分钟内无弹幕，按顺序排查：

```bash
# a) 服务端是否收到请求？
docker compose logs --tail=100 getapp-danmu | grep danmu

# b) 是否命中资源站？
#    日志出现「资源站 -> 剧名=... 第1季 第1集」= 命中
#    日志什么都没出现 = url 没传进来，回去检查第四步的自定义域名

# c) 直接手工模拟 APP 的请求
curl -G "http://127.0.0.1:12381/" --data-urlencode "ac=dm" \
     --data-urlencode "url=https://jimaoys94.com/public/playback/c365c8cf36c195bd4aa367677ad84ae8/smart.m3u8" \
     --data-urlencode "douban_id=35633634"
```

---

## 六、第五步：添加 / 替换资源站

资源站 CDN 域名会不定期变更（`jimaoys80` → `81` → `82` …）。本服务提供两种更新方式，**都不需要重启**。

### 6.1 方式一：改配置文件（自动热重载）

```bash
cd /opt/logvar-getapp-docker
vim data/config.json     # 修改 resource_hosts 数组
# 保存后 15 秒内自动生效
docker compose logs --tail=5 getapp-danmu
# -> [config] 检测到 ".../config.json" 变更，执行热重载
```

> 热重载间隔由 `watch_interval_sec` 控制（默认 15 秒）。
> 由于是宿主机编辑挂载目录，容器内文件 mtime 会同步更新，故能触发检测。

### 6.2 方式二：/admin 接口（立即生效，推荐脚本化）

前置：`data/config.json` 中 `admin_enabled: true` 且 `admin_token` 已设置。

```bash
BASE=http://127.0.0.1:12381
TOKEN=换成你自己的复杂口令
H="-H X-Admin-Token:$TOKEN"

# 查看当前资源站
curl $H $BASE/admin/sources

# 新增（实时生效 + 自动落盘）
curl $H -H "Content-Type: application/json" \
  -X POST -d '{"resource_hosts":["jimaoys83.com","https://jimaoys82.com/"]}' \
  $BASE/admin/sources

# 删除
curl $H -H "Content-Type: application/json" \
  -X DELETE -d '{"resource_hosts":["https://jimaoys82.com"]}' \
  $BASE/admin/sources

# 整体替换
curl $H -H "Content-Type: application/json" \
  -X PUT -d '{"resource_hosts":["jimaoys95.com","jimaoys94.com"]}' \
  $BASE/admin/sources

# 修改选源优先级（想让 B 站弹幕优先就调顺序）
curl $H -H "Content-Type: application/json" \
  -X POST -d '{"source_priority":["bilibili","tencent","iqiyi"]}' \
  $BASE/admin/priority

# 更换 LogVar 上游
curl $H -H "Content-Type: application/json" \
  -X POST -d '{"logvar_base":"https://your-logvar/xxx","logvar_token":""}' \
  $BASE/admin/logvar

# 手动触发整体配置重载
curl $H -X POST $BASE/admin/reload
```

域名写法很宽松，程序会自动规范化：

| 你写的 | 存进去的 |
|---|---|
| `jimaoys95.com` | `https://jimaoys95.com` |
| `https://Jimaoys95.com/` | `https://jimaoys95.com` |
| `http://jimaoys95.com/path` | `http://jimaoys95.com` |

**记得同步**：新增资源站时，也要把域名加到 Getapp 后台的“自定义域名”里，否则 APP 仍不会把该站的播放地址提交过来。

### 6.3 自动化：定时同步资源站

如果资源站域名经常换，可写个 cron 定期从采集源拉取域名并推给 `/admin`：

```bash
# 每 30 分钟把一组候选域名整体替换进去
*/30 * * * * curl -s -H "X-Admin-Token: <TOKEN>" -H "Content-Type: application/json" \
  -X PUT -d "$(curl -s https://your-source/domains.json)" \
  http://127.0.0.1:12381/admin/sources
```

---

## 七、第六步：苹果CMS 直连抓取（可选）

到这一步，服务已经能给 APP 正常返回弹幕了。本节解决另一类需求：**手上只有视频 ID 或详情页 URL，想直接抓 m3u8 / 弹幕**（例如在浏览器里验证，或用脚本批量导出某片全部集数）。

入口是 `?ac=cms`：先请求详情页解析出全部分集的 `sid/nid`，再逐集请求播放页解析内联 `player_aaaa` 拿到 m3u8，最后交给与 APP 完全相同的弹幕匹配链路。

### 7.1 配置站点地址

编辑 `data/config.json`（热重载，无需重启）：

```json
{
  "cms_base_url": "https://www.501710491.xyz",
  "cms_concurrency": 4,
  "cms_timeout_ms": 30000,
  "cms_all_episodes": true,
  "cms_max_episodes": 0
}
```

| 字段 | 说明 |
|---|---|
| `cms_base_url` | 苹果CMS 站点根地址，**不带** `/index.php`；若每次都用 `url` 传完整地址可留空 |
| `cms_concurrency` | 逐集抓播放页的并发数，站点限速明显时调小 |
| `cms_timeout_ms` | 单次上游请求（详情页/播放页）超时（毫秒）；单次抓取整体预算另有兜底（×4，下限 30s、上限 3min） |
| `cms_all_episodes` | 默认是否抓取全部分集 |
| `cms_max_episodes` | 单次最多抓几集，`0` 不限（大剧集建议设 10~20 控制耗时） |

### 7.2 跑一次抓取

```sh
BASE=http://127.0.0.1:12381

# 先只抓 m3u8，确认链路通
curl "$BASE/?ac=cms&id=2421&danmu=0"

# 确认无误后再带上弹幕匹配
curl "$BASE/?ac=cms&id=2421"
```

预期：返回中 `cms.episodes[]` 每集都有 `m3u8` 且 `ok=true`；`danum` 大于 0 说明弹幕匹配成功。

### 7.3 参数速查

| 参数 | 作用 |
|---|---|
| `id` / `url` | 二选一：视频 ID，或详情页 URL（传播放页 URL 亦可） |
| `sid` / `nid` | 只处理某个播放源 / 某一集 |
| `all=1` | 强制抓全量集（覆盖配置） |
| `match_all=1` | 合并返回全部集的弹幕，而非仅“选中集” |
| `danmu=0` | 只抓 m3u8，不匹配弹幕 |

### 7.4 抓不到时怎么排查

| 现象 | 先看这里 |
|---|---|
| `无法从入参解析出视频 ID` | `url` 是否为详情页/播放页地址（含 `/id/{数字}` 或 `?id={数字}`） |
| 详情页报 `upstream HTTP error status 5xx` | 站点是否被 Cloudflare 拦；先用浏览器访问同一 URL 确认站点可用 |
| `详情页未解析出任何分集` | 该站分集列表是否由 JS 动态渲染（本项目只解析服务端渲染的链接） |
| 某集 `encrypt=1` | 站点启用了加密播放地址，需补 `cmsDecryptHook` |
| m3u8 有但弹幕为 0 | 该片在 LogVar 上游没库，或资源站域名未加入 `resource_hosts` |

> 完整字段与实现细节见 [`cms-direct-crawl.md`](cms-direct-crawl.md)，接口示例见 README 第八节。

---

## 八、第七步：日常运维

### 8.1 常用命令

```bash
cd /opt/logvar-getapp-docker

docker compose ps                      # 状态（含 healthy）
docker compose logs -f --tail=100      # 实时日志
docker compose restart                 # 重启
docker compose down                    # 停止并删除容器（配置在 data/ 里不会丢）
docker compose up -d --build           # 改代码后重建
```

### 8.2 健康检查

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:12381/health   # 200
docker inspect --format '{{.State.Health.Status}}' getapp-danmu          # healthy
```

### 8.3 备份与迁移

需要备份的只有 `data/` 目录（内含 `config.json`）：

```bash
tar czf getapp-danmu-backup-$(date +%F).tar.gz data/
```

迁移到新机器：拷 `data/` + 工程目录 → `docker compose up -d --build`。

### 8.4 升级

```bash
docker compose down
# 替换 *.go / Dockerfile 等文件（保留 data/ 不动）
docker compose up -d --build
```

### 8.5 安全建议

| 项 | 建议 |
|---|---|
| `/admin` 暴露面 | 默认关闭；要用则设强口令，并尽量只在内网或反代鉴权后开放 |
| 端口 | 若 Getapp 与本服务同机，可只监听 `127.0.0.1`（改为 `-p 127.0.0.1:12381:12381`） |
| 容器权限 | 镜像已用非 root 用户 `app` 运行 |

---

## 九、排障 FAQ

| 现象 | 原因与处理 |
|---|---|
| `docker compose up` 报端口占用 | 改 `docker-compose.yml` 的端口映射（如 `12382:12381`）与 Getapp 里填的地址 |
| `/health` 正常，但 APP 无弹幕 | 九成是 Getapp 的“自定义域名”没加资源站域名，导致 `url` 没传进来。看日志有没有 `资源站 -> 剧名=` |
| 日志有 `资源站分享页解析失败` | 该 `resource_hosts` 主机不可达或该 hash 在分享页不存在。换/加资源站域名 |
| 日志有 `logvar returned status 5xx` | LogVar 上游抖动或换了地址。用 `/admin/logvar` 更换上游 |
| `danum` 很大但 APP 卡 | 调小 `max_danmu`（如 `3000`），热重载生效 |
| 某些老剧没弹幕 | LogVar 无该片数据，属正常，换官方源地址测试可对比确认 |
| 改了 `config.json` 没生效 | 确认改的是**挂载卷内**的 `data/config.json`；确认间隔已过（默认 15s）；或用 `/admin/reload` |
| 改了 `listen` 没生效 | 监听端口不支持热切换，需重启容器 |
| 环境变量改了 `resource_hosts` 但 `/admin` 改不动 | 环境变量优先级最高，会覆盖文件与接口。把该环境变量从 compose 中删掉 |
| iOS APP 看不到弹幕 | iOS ATS 要求 https，需给弹幕域名配 SSL 反代（见下） |
| 容器启动即退出 | `docker compose logs` 看错误；常见是 `config.json` JSON 语法错误（如多余的逗号） |

---

## 十、进阶：反向代理与 HTTPS

以 Nginx 为例（域名 `dm.example.com` → 本服务）：

```nginx
server {
    listen 443 ssl http2;
    server_name dm.example.com;

    ssl_certificate     /etc/nginx/ssl/dm.example.com.pem;
    ssl_certificate_key /etc/nginx/ssl/dm.example.com.key;

    location / {
        proxy_pass         http://127.0.0.1:12381;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_read_timeout 120s;   # 单集弹幕可能几万条，留足时间
        proxy_buffering    off;
    }
}
```

配好后，Getapp 后台填 `https://dm.example.com/?ac=dm`。

宝塔面板用户：新建站点 → 反向代理 → 目标 `http://127.0.0.1:12381` → 申请 Let's Encrypt 证书即可。

> `/admin` 若也走公网域名，建议在 Nginx 层再加一层 IP 白名单或 basic auth。

---

## 十一、完整链路回顾

```
APP 播放某集
   │  GET /?ac=dm&url=<m3u8或官方地址>&douban_id=xxx
   ▼
本服务 handler.go
   ├─ 官方平台地址 ──> LogVar /api/v2/comment?url= ──> 弹幕
   ├─ 资源站 m3u8  ──> 取32位hash ──> 分享页 /s/<hash> ──> 剧名/季/集
   │                                     └─> LogVar 搜索+打分选源+选集 ──> 弹幕
   └─ 豆瓣ID ──> 豆瓣标题 ──> LogVar ──> 弹幕
   ▼
transform.go 转换为 Getapp danmuku 格式
   ▼
APP 渲染弹幕
```

另有「无 APP」链路（`?ac=cms`）：

```
只给视频 ID 或详情页 URL
   │  GET /?ac=cms&id=2421
   ▼
cms.go  详情页 ─> 全部分集(sid/nid) ─> 逐集播放页 ─> player_aaaa ─> m3u8（encrypt=1 走解密钩子）
   ▼
handle_cms.go  逐集复用上面同一条弹幕链路（hash 反查 ─> LogVar；失败回退详情页剧名）
   ▼
返回 code/name/danum/danmuku + cms 明细（sources[] / episodes[] / ok_count / total）
```

至此全部完成。需要新增/替换资源站时，直接改 `data/config.json` 或调 `/admin/sources` 即可，服务无需重启。
*（内容由AI生成，仅供参考）*
*（内容由AI生成，仅供参考）*
