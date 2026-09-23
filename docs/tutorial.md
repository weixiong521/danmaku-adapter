---
AIGC:
    Label: "1"
    ContentProducer: 001191440300708461136T1XGW3
    ProduceID: 72bedbacb9db265375b893d98c8f8513_5ea0955ab78c11f199d2525400393706
    ReservedCode1: Do6hj4odLWKIrN91XP3qRfyd71QKFP72IoDDv2z7qRzd5SHtw8aOPvOjutzP7yO/LYjaIToPUwrRLCMYLS1Mp8WmDEIvZ0yWGVtKBkbwdBmCmNe3rEJJE2XcDH2VhX5hupM9IupCYbY4RmRyNDm6y34pZSYT7s3VFUj6pq3mFxQ8Mn6m9n2gPsXhg/M=
    ContentPropagator: 001191440300708461136T1XGW3
    PropagateID: 72bedbacb9db265375b893d98c8f8513_5ea0955ab78c11f199d2525400393706
    ReservedCode2: Do6hj4odLWKIrN91XP3qRfyd71QKFP72IoDDv2z7qRzd5SHtw8aOPvOjutzP7yO/LYjaIToPUwrRLCMYLS1Mp8WmDEIvZ0yWGVtKBkbwdBmCmNe3rEJJE2XcDH2VhX5hupM9IupCYbY4RmRyNDm6y34pZSYT7s3VFUj6pq3mFxQ8Mn6m9n2gPsXhg/M=
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
7. [第六步：日常运维](#七第六步日常运维)
8. [排障 FAQ](#八排障-faq)
9. [进阶：反向代理与 HTTPS](#九进阶反向代理与-https)

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

## 七、第六步：日常运维

### 7.1 常用命令

```bash
cd /opt/logvar-getapp-docker

docker compose ps                      # 状态（含 healthy）
docker compose logs -f --tail=100      # 实时日志
docker compose restart                 # 重启
docker compose down                    # 停止并删除容器（配置在 data/ 里不会丢）
docker compose up -d --build           # 改代码后重建
```

### 7.2 健康检查

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:12381/health   # 200
docker inspect --format '{{.State.Health.Status}}' getapp-danmu          # healthy
```

### 7.3 备份与迁移

需要备份的只有 `data/` 目录（内含 `config.json`）：

```bash
tar czf getapp-danmu-backup-$(date +%F).tar.gz data/
```

迁移到新机器：拷 `data/` + 工程目录 → `docker compose up -d --build`。

### 7.4 升级

```bash
docker compose down
# 替换 *.go / Dockerfile 等文件（保留 data/ 不动）
docker compose up -d --build
```

### 7.5 安全建议

| 项 | 建议 |
|---|---|
| `/admin` 暴露面 | 默认关闭；要用则设强口令，并尽量只在内网或反代鉴权后开放 |
| 端口 | 若 Getapp 与本服务同机，可只监听 `127.0.0.1`（改为 `-p 127.0.0.1:12381:12381`） |
| 容器权限 | 镜像已用非 root 用户 `app` 运行 |

---

## 八、排障 FAQ

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

## 九、进阶：反向代理与 HTTPS

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

## 十、完整链路回顾

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

至此全部完成。需要新增/替换资源站时，直接改 `data/config.json` 或调 `/admin/sources` 即可，服务无需重启。
*（内容由AI生成，仅供参考）*
