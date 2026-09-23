# GitHub Actions 自动构建并推送 Docker Hub

本文档说明如何把本项目接入 GitHub Actions：**代码推送到 GitHub 后自动构建 Docker 镜像并推送到 Docker Hub**，支持多架构（可选）、按 tag 触发，并说明别人如何 `docker pull` 与运行。

---

## 一、整体流程

```
本地仓库
  │
  ├── push 分支 main / 提交 PR ──► docker-build-check.yml
  │                                  └─ 仅构建 linux/amd64 校验（不推送、不登录）
  │
  └── push tag v1.2.3 ──────────► docker-publish.yml
                                     ├─ QEMU + Buildx（默认 linux/amd64,linux/arm64）
                                     ├─ 按 tag 生成镜像标签（1.2.3 / 1.2 / 1 / v1.2.3 / latest）
                                     ├─ 用 secrets 登录 Docker Hub
                                     └─ push 到 <DOCKERHUB_USERNAME>/logvar-getapp-danmu
```

涉及文件：

| 文件 | 作用 |
|---|---|
| `.github/workflows/docker-publish.yml` | 发布流程：tag 触发 / 手动触发，多架构构建 + 推送 Docker Hub |
| `.github/workflows/docker-build-check.yml` | 校验流程：PR 与 main 提交时只构建不推送，提前暴露构建错误 |
| `.gitignore` | 防止 `data/`、`config.json`（含 token）等被提交 |

---

## 二、前置准备

### 2.1 获取 Docker Hub Access Token

1. 登录 [hub.docker.com](https://hub.docker.com/)，若还没有账号先注册。
   - **用户名必须全小写**（Docker 镜像名要求全小写，含大写会导致 `invalid reference format`）。
2. 右上角头像 → **Account Settings** → **Security** → **New Access Token**。
3. 描述随意（如 `github-actions`），权限选择 **Read & Write**（必须包含写权限，否则推送会被拒绝）。
4. 点击 **Generate**，**立即复制**生成的 token（形如 `dckr_pat_xxxxxxxx`），页面关闭后无法再查看。
   - ⚠ 这里要用 **Access Token**，不要用登录密码（Docker Hub 已不支持密码登录 CI）。

### 2.2 在 GitHub 仓库配置 Secrets

进入 GitHub 仓库页面：

**Settings → Secrets and variables → Actions → Secrets 选项卡 → New repository secret**

添加两条（名称必须完全一致）：

| Secret 名称 | 值 | 说明 |
|---|---|---|
| `DOCKERHUB_USERNAME` | 你的 Docker Hub 用户名 | 例如 `zhangsan`，全小写 |
| `DOCKERHUB_TOKEN` | 上一步生成的 Access Token | 例如 `dckr_pat_xxxxxxxx` |

> workflow 中使用的是 **Repository secrets**（未声明 `environment`）。如果你把 secret 建在 Environment 下，需要另在 job 中声明 `environment:`，否则读取不到。
>
> Secret 一旦保存无法再查看原文，只能重新设置。泄露后请立即在 Docker Hub 删除该 token 并重建。

### 2.3 确认镜像名（可选）

`docker-publish.yml` 顶部的 `env` 区块：

```yaml
env:
  IMAGE_NAME: logvar-getapp-danmu          # 镜像名，可按需修改
  BUILD_CONTEXT: .                          # 构建上下文：仓库根目录
  DOCKERFILE_PATH: ./Dockerfile             # 与 docker-compose.yml 的 dockerfile 字段一致
  DEFAULT_PLATFORMS: linux/amd64,linux/arm64
```

最终推送地址为：`${DOCKERHUB_USERNAME}/${IMAGE_NAME}`，例如 `zhangsan/logvar-getapp-danmu`。

---

## 三、触发方式

| 触发方式 | 操作 | 行为 |
|---|---|---|
| **推送 tag（推荐，发布正式版）** | `git tag v1.2.3 && git push origin v1.2.3` | 多架构构建 + 推送到 Docker Hub，自动打 `1.2.3`/`1.2`/`1`/`v1.2.3`/`latest` |
| **手动触发** | 仓库 **Actions → Docker Publish → Run workflow** | 可选架构、可选是否推送，适合调试与补发 |
| **main 分支推送**（默认关闭） | 取消 `docker-publish.yml` 中 `on.push.branches` 注释 | 构建并推送 `edge` 标签（表示最新开发版） |
| PR / main 提交校验 | 自动 | 仅构建 `linux/amd64`，不登录、不推送（见 `docker-build-check.yml`） |

### 3.1 发布新版本的完整命令

```bash
# 1) 先把代码提交并推送到分支
git add .
git commit -m "feat: 支持 xxx"
git push origin main

# 2) 打 tag 并推送（这一步才会触发镜像发布）
git tag v1.2.3
git push origin v1.2.3
```

注意事项：

- tag **必须以 `v` 开头**，否则不会触发 workflow（`on.push.tags: ['v*']`）。
- 只推送分支不会发布镜像（除非手动打开了 main 触发）。
- 预发布 tag（如 `v1.2.3-rc1`）也会发布，但**不会**覆盖 `latest`。
- 想重跑：Actions 页面选中对应 run → **Re-run all jobs**；或删除远端 tag 后重推。

### 3.2 手动触发参数

| 参数 | 默认 | 说明 |
|---|---|---|
| `platforms` | `linux/amd64,linux/arm64` | 目标架构，单选：双架构 / 仅 amd64 / 仅 arm64 |
| `push_image` | `true` | `false` 表示只构建校验，不登录 Docker Hub、不推送 |
| `image_tag` | 空 | 额外自定义标签，例如填 `test1` 会同时推送 `:test1` |

> 手动触发且未填 `image_tag` 时，会自动生成 `manual-<run号>` 标签，避免"无标签可推"报错。

---

## 四、镜像命名与标签规则

**镜像名**：`<DOCKERHUB_USERNAME>/logvar-getapp-danmu`
**标签来源**：由 `docker/metadata-action` 依据触发事件自动生成。

| 触发源 | 生成的标签 | 示例 |
|---|---|---|
| tag `v1.2.3` | `1.2.3`、`1.2`、`1`、`v1.2.3`、**`latest`** | `zhangsan/logvar-getapp-danmu:1.2.3` |
| tag `v1.2.3-rc1`（预发布） | `1.2.3-rc1`、`v1.2.3-rc1`（**无 latest**） | 用于灰度/测试 |
| tag `v1.0`（非标准三段 semver） | `v1.0`（保留原始标签名兜底） | 建议统一用 `vX.Y.Z` |
| main 分支（需先启用） | `edge` | 滚动开发版 |
| 手动触发 | 自定义 `image_tag` 或 `manual-<run号>` | |

推荐给使用者的稳定引用：

- `:latest` —— 最新正式版
- `:1`、`:1.2` —— 大版本/次版本滚动，可自动获得该系列的修复
- `:1.2.3` —— 精确锁定版本（生产环境建议）

多架构说明：一次构建产出的所有标签共用同一个 **manifest list**，使用者 `docker pull` 时 Docker 会按自身 CPU 架构自动挑选对应镜像，无需手动指定。

---

## 五、别人如何使用

### 5.1 拉取镜像

```bash
# 最新正式版（自动匹配当前机器架构）
docker pull zhangsan/logvar-getapp-danmu:latest

# 精确版本
docker pull zhangsan/logvar-getapp-danmu:1.2.3

# 指定架构（跨架构拉取时才需要）
docker pull --platform linux/arm64 zhangsan/logvar-getapp-danmu:1.2.3

# 查看镜像包含哪些架构
docker manifest inspect zhangsan/logvar-getapp-danmu:1.2.3
```

### 5.2 首次运行（docker run）

```bash
# 准备数据目录（配置与持久化数据都放这里）
mkdir -p /opt/getapp-danmu/data
cd /opt/getapp-danmu

docker run -d \
  --name getapp-danmu \
  --restart unless-stopped \
  -p 12381:12381 \
  -v "$PWD/data:/app/data" \
  -e TZ=Asia/Shanghai \
  -e ADMIN_ENABLED=true \
  -e ADMIN_TOKEN=换成你的强口令 \
  zhangsan/logvar-getapp-danmu:latest
```

- 首次启动时，入口脚本会用内置的 `config.example.json` 在 `/app/data/config.json` 生成配置文件。
- 端口 `12381` 为服务监听端口；健康检查地址为 `http://127.0.0.1:12381/health`。
- 若宿主机 12381 被占用，改左侧映射即可，如 `-p 23456:12381`。

### 5.3 docker compose 部署

```yaml
services:
  getapp-danmu:
    image: zhangsan/logvar-getapp-danmu:1.2.3
    container_name: getapp-danmu
    restart: unless-stopped
    ports:
      - "12381:12381"
    environment:
      TZ: Asia/Shanghai
      LISTEN: ":12381"
      CONFIG_PATH: "/app/data/config.json"
      ADMIN_ENABLED: "true"
      ADMIN_TOKEN: "换成你的强口令"
      WATCH_INTERVAL_SEC: "15"
    volumes:
      - ./data:/app/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:12381/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 5s
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

启动：`docker compose up -d`；更新版本：改 `image` 的 tag 后 `docker compose pull && docker compose up -d`。

### 5.4 环境变量与热更新（重要）

配置优先级：**环境变量 > `/app/data/config.json` > 内置默认值**。

⚠ **环境变量优先级最高且每次热重载都会重新生效**。也就是说，一旦把资源站类配置写进 `environment`，之后无论改 `config.json` 还是调 `/admin` 接口，都会被环境变量覆盖回去。

因此建议按下表划分：

| 变量 | 建议写在 env | 说明 |
|---|---|---|
| `TZ` | ✅ | 时区，如 `Asia/Shanghai` |
| `LISTEN` | ✅ | 监听地址，默认 `:12381` |
| `CONFIG_PATH` | ✅ | 配置文件路径，默认 `/app/data/config.json` |
| `ADMIN_ENABLED` / `ADMIN_TOKEN` | ✅ | 管理接口开关与令牌，生产环境务必改强口令 |
| `WATCH_INTERVAL_SEC` | ✅ | 配置文件监听间隔（秒） |
| `RESOURCE_HOSTS` | ❌ | 资源站列表，放 `config.json` 或用 `/admin` 热更新 |
| `LOGVAR_BASE` / `LOGVAR_TOKEN` | ❌ | 上游地址与令牌，同上 |
| `SOURCE_PRIORITY` | ❌ | 选源优先级，同上 |
| `MAX_DANMU` / `HTTP_TIMEOUT_MS` / `RESOLVE_TTL_SEC` | ❌ | 运行参数，改 `config.json` 即可 |

即：**固定不变的运行参数用 env，需要热更的资源站类配置用文件或 `/admin`。**

---

## 六、workflow 与本项目构建方式的对应关系

| workflow 配置 | 对应本项目 | 说明 |
|---|---|---|
| `context: .` | 仓库根目录即本项目目录 | Dockerfile 中 `COPY *.go ./`、`COPY config.example.json` 均相对根目录 |
| `file: ./Dockerfile` | 与 `docker-compose.yml` 的 `dockerfile: Dockerfile` 一致 | 同一份多阶段构建（`golang:1.21-alpine` 编译 → `alpine` 运行） |
| `platforms` | Dockerfile 本身与架构无关 | 由 buildx + QEMU 交叉编译 arm64 |
| 未使用 `build-args` | Dockerfile 未声明 `ARG` | 若日后想注入版本号，需在 Dockerfile 补 `ARG`/`LABEL` 后再在此处传参 |
| `cache-from/to: type=gha` | Go 依赖层 | publish 与 check 使用不同 `scope`，互不干扰 |
| `provenance: false` | — | 关闭附加证明清单，避免 Docker Hub 上出现 `unknown/unknown` 平台条目 |

⚠ 仓库存放位置：**`.github/` 必须位于仓库根目录**，且与 `Dockerfile`、`*.go` 同级（即整个 `logvar-getapp-docker` 目录作为仓库根）。
若仓库根是整个父目录、项目放在子目录下，需把两处改为：

```yaml
context: ./logvar-getapp-docker
file: ./logvar-getapp-docker/Dockerfile
```

---

## 七、首次上线检查清单

- [ ] 仓库根目录 = 本项目目录（`Dockerfile`、`*.go`、`.github/` 同级）
- [ ] 已在 GitHub 配置 `DOCKERHUB_USERNAME`、`DOCKERHUB_TOKEN` 两个 secrets
- [ ] Docker Hub 上的镜像仓库已创建，或允许自动创建（首次推送会自动建）
- [ ] `data/`、`config.json` 没有被提交（`.gitignore` 已兜底）
- [ ] 代码已 push 到远端后再打 tag
- [ ] 镜像仓库设为 **Public**，否则别人 `docker pull` 需要先登录
- [ ] 本机无 Docker 环境时，先跑一次 **Docker Build Check**（或手动触发 `push_image=false`）验证构建通过

---

## 八、常见问题排查

| 现象 | 原因 | 处理 |
|---|---|---|
| `denied: requested access to the resource is denied` | Token 无写权限，或用户名与 token 所属账号不一致 | 重建 Token 时选 **Read & Write**，核对两个 secret |
| `unauthorized: incorrect username or password` / 401 | Token 已过期或被删除 | 重新生成 Token 并更新 `DOCKERHUB_TOKEN` |
| `invalid reference format` | 用户名含大写字母或非法字符 | Docker Hub 用户名必须全小写 |
| 别人 pull 后报 `no matching manifest for linux/arm64` | 只构建了 amd64 | 手动触发并把 `platforms` 选为双架构后重新发布 |
| Docker Hub 上出现 `unknown/unknown` 平台 | buildx 默认附加 provenance 清单 | workflow 已设 `provenance: false`，无需处理 |
| workflow 没被触发 | tag 不以 `v` 开头，或只推了分支 | 用 `vX.Y.Z` 形式打 tag 并 `git push origin <tag>` |
| 报错 `no such file or directory: Dockerfile` | 仓库根不是项目目录 | 按第六节调整 `context` / `file` 路径 |
| arm64 构建非常慢 | QEMU 模拟编译 | 只发 `linux/amd64`，或改用 arm 原生 runner |
| 容器起来了但没生成配置 | `/app/data` 未挂载或不可写 | 挂载卷后重启，确认目录权限 |
| 改了 `config.json` 不生效 | 同名环境变量优先级更高 | 从 `environment` 中移除该变量，改用文件或 `/admin` |

---

## 九、可选进阶

1. **自动创建 GitHub Release**：在发布 job 末尾追加 `softprops/action-gh-release@v2`。
2. **同步 README 到 Docker Hub 仓库页**：追加 `peter-evans/dockerhub-description@v4`，需 `DOCKERHUB_PASSWORD` 与 `DOCKERHUB_REPOSITORY` 配置。
3. **同时推送 GitHub Container Registry (GHCR)**：在 `metadata-action` 的 `images` 中加入 `ghcr.io/${{ github.repository }}`，并授予 `packages: write` 权限。
4. **镜像漏洞扫描**：追加 `aquasecurity/trivy-action` 步骤，对构建产物做 CVE 扫描。
