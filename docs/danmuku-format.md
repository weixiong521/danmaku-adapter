---
AIGC:
    Label: "1"
    ContentProducer: 001191440300708461136T1XGW3
    ProduceID: 72bedbacb9db265375b893d98c8f8513_5d0eaa83b78c11f199d2525400393706
    ReservedCode1: 8Qeyvh3VjD+e3qDosygtwAYmiBODFpcweC82QsKy/BoW3qVdineg5r1mw8snF3KNLo7iz+/w3BrSV0etf5oaPCPYkV3u+syHsXO0uehdP+6EDURa6AfApkMjtATnOeg/pZiwi+sTyAqNkDVU8j30Him0zTxlnq1FVvt8kjSt/fHY8kSv3JVx2+wHqno=
    ContentPropagator: 001191440300708461136T1XGW3
    PropagateID: 72bedbacb9db265375b893d98c8f8513_5d0eaa83b78c11f199d2525400393706
    ReservedCode2: 8Qeyvh3VjD+e3qDosygtwAYmiBODFpcweC82QsKy/BoW3qVdineg5r1mw8snF3KNLo7iz+/w3BrSV0etf5oaPCPYkV3u+syHsXO0uehdP+6EDURa6AfApkMjtATnOeg/pZiwi+sTyAqNkDVU8j30Him0zTxlnq1FVvt8kjSt/fHY8kSv3JVx2+wHqno=
---

# 弹幕库（接口）字段说明

本服务对外暴露 **Getapp 协议** 的弹幕接口，对内消费 **LogVar（弹弹play）** 数据。
下面完整列出两端的字段定义：参数名、类型、说明、示例。

---

## 一、本服务对外接口

### 1.1 基本信息

| 项 | 值 |
|---|---|
| 方法 | `GET`（支持 `OPTIONS` 预检） |
| 路径 | `/`、`/dm`、`/danmu` 三者等价 |
| 完整形态 | `http://<host>:12381/?ac=dm&url=<播放地址>&douban_id=<豆瓣ID>` |
| 返回 | `application/json; charset=utf-8` |

### 1.2 请求参数

| 参数名 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|
| `ac` | string | 否 | Getapp 固定传 `dm`，本服务不校验，仅作标识 | `dm` |
| `url` | string | 二选一 | 影片播放地址。支持官方平台页地址，或资源站 `.m3u8` 播放地址 | `https%3A%2F%2Fv.qq.com%2Fx%2Fcover%2F...` |
| `douban_id` | string | 二选一 | 豆瓣条目 ID（纯数字），`url` 无效时的兜底通道 | `35633634` |
| `ep` / `episode` | int | 否 | 集数兜底参数，**并非 Getapp 默认字段**，仅用于外部自测/自定义对接 | `3` |

> ⚠️ 中文/特殊字符需 URL 编码。参数名区分大小写，请按上表原样使用。

### 1.3 响应结构（顶层）

| 参数名 | 类型 | 说明 | 示例 |
|---|---|---|---|
| `code` | int | 状态码，`1` 表示成功（无论是否有弹幕） | `1` |
| `name` | string | 命中的剧名（来自 LogVar 匹配结果；无法匹配时为空串） | `"庆余年 第一季(2019)【国产剧】from tencent"` |
| `danum` | int | 弹幕条数，等于 `danmuku` 数组长度 | `40457` |
| `danmuku` | array | 弹幕二维数组，每个元素是一条弹幕（8 个字符串元素） | 见 1.4 |

### 1.4 `danmuku` 单条弹幕字段（**核心**）

每条弹幕是一个**固定 8 元素**的字符串数组，顺序不可变：

| 序号 | 参数名 | 类型 | 说明 | 示例 |
|---|---|---|---|---|
| 1 | `time` | string | 弹幕出现时间，单位秒，保留两位小数，相对视频开头 | `"12.50"` |
| 2 | `position` | string | 弹幕位置/运动方式，取值 `right` / `top` / `bottom` / `left` | `"right"` |
| 3 | `color` | string | 弹幕颜色，`#RRGGBB` 十六进制 | `"#FF1964"` |
| 4 | `reserve` | string | 保留字段，Getapp 固定为 `"0"` | `"0"` |
| 5 | `content` | string | 弹幕文本内容 | `"前方高能"` |
| 6 | `ip` | string | 发送者 IP，Getapp 格式占位，固定 `"127.0.0.1"` | `"127.0.0.1"` |
| 7 | `timestamp` | string | 弹幕发送时间戳（Unix 秒）。LogVar 未提供，本服务填当前时间 | `"1790185274"` |
| 8 | `fontSize` | string | 字号（含单位），由配置 `font_size` 决定 | `"24px"` |

**position 取值对照**

| 值 | 含义 |
|---|---|
| `right` | 从右向左滚动（普通弹幕） |
| `left` | 从左向右滚动（少见） |
| `top` | 顶部固定 |
| `bottom` | 底部固定 |

### 1.5 完整响应示例

```json
{
  "code": 1,
  "name": "庆余年 第一季(2019)【国产剧】from tencent",
  "danum": 3,
  "danmuku": [
    ["0.00",   "right",  "#FF1964", "0", "前方高能",     "127.0.0.1", "1790185274", "24px"],
    ["5.20",   "top",    "#FFFFFF", "0", "这剧真的好看", "127.0.0.1", "1790185274", "24px"],
    ["12.50",  "right",  "#FFFF00", "0", "打卡",         "127.0.0.1", "1790185274", "24px"]
  ]
}
```

### 1.6 异常/边界响应

| 场景 | 响应 |
|---|---|
| 无参数 | `{"code":1,"name":"","danum":0,"danmuku":[]}` |
| 资源站分享页不可达 | 同上（`name` 为空，`danum` 为 0），日志会打印失败原因 |
| LogVar 上游 5xx / 超时 | 同上，日志记录 `logvar returned status 5xx` |
| `/admin` 未授权 | HTTP `401` + `unauthorized: 管理接口未开启或令牌错误` |
| `/admin` 未知路径 | HTTP `404` |

> 设计上**任何失败都返回 HTTP 200 + 空弹幕**，避免 APP 端因非 200 而报错或反复重试。

---

## 二、LogVar（上游）字段说明

### 2.1 搜索动漫

| 项 | 值 |
|---|---|
| 路径 | `GET {logvar_base}[/{token}]/api/v2/search/anime?keyword=<剧名>` |

| 参数名 | 类型 | 说明 |
|---|---|---|
| `animeId` | int64 | 条目 ID（后续查详情用） |
| `animeTitle` | string | 剧名，通常带 `第X季`、`(年份)`、`【类型】from 来源` 等后缀 |
| `type` | string | 类型，如 `tvseries` / `movie` |
| `episodeCount` | int | 总集数 |
| `source` | string | 数据来源，如 `tencent` / `iqiyi` / `bilibili` / `youku` / `mango` / `migu` / `renren` / `kan360` / `douban` / `vod` |
| `aliases` | []string | 别名列表 |

### 2.2 剧集详情

| 项 | 值 |
|---|---|
| 路径 | `GET {logvar_base}[/{token}]/api/v2/bangumi/{animeId}` |

| 参数名 | 类型 | 说明 |
|---|---|---|
| `episodeId` | number | 集 ID（查弹幕用），**可能超过 int32，需按字符串处理** |
| `episodeNumber` | string | 集号，如 `"1"` |
| `episodeTitle` | string | 集标题 |
| `url` | string | 该集的官方播放地址 |

### 2.3 弹幕列表

| 项 | 值 |
|---|---|
| 路径 | `GET {logvar_base}[/{token}]/api/v2/comment/{episodeId}`，或 `GET /api/v2/comment?url=<官方播放地址>` |

| 参数名 | 类型 | 说明 |
|---|---|---|
| `cid` | int64 | 弹幕 ID |
| `p` | string | **弹幕参数**，逗号分隔四段：`时间,模式,颜色,来源`，见 2.4 |
| `m` | string | 弹幕文本 |
| `count` | int | 总条数 |

### 2.4 `p` 字段（四段）与转换映射

| 段序 | 含义 | 类型 | 取值 | 示例 |
|---|---|---|---|---|
| 1 | 时间（秒） | float | 相对视频开头 | `12.50` |
| 2 | 模式 | int | `1`=滚动 `2`=顶部 `3`=底部 `4`=反向 | `1` |
| 3 | 颜色 | int | **十进制 RGB 整数**，如 `16777215` = `#FFFFFF` | `16711680` |
| 4 | 来源 | string | 形如 `[qq]` / `[bilibili]`，可缺省 | `[qq]` |

**转换映射表（LogVar `p` → Getapp `danmuku`）**

| Getapp 字段 | 由 LogVar 何而来 | 转换规则 |
|---|---|---|
| `time` | `p[0]` | 解析为 float 后 `%.2f` |
| `position` | `p[1]` | `1→right`、`2→top`、`3→bottom`、`4→left`；其余默认 `right` |
| `color` | `p[2]` | 十进制整数转 `#RRGGBB`；非法值用 `default_color` |
| `reserve` | — | 固定 `"0"` |
| `content` | `m` | 原样 |
| `ip` | — | 固定 `"127.0.0.1"` |
| `timestamp` | — | 本服务处理时刻的 Unix 秒 |
| `fontSize` | — | 配置 `font_size`（默认 `24px`） |

**颜色换算示例**

| LogVar `p[2]` | 计算 | Getapp `color` |
|---|---|---|
| `16777215` | `0xFFFFFF` | `#FFFFFF` |
| `16711680` | `0xFF0000` | `#FF0000` |
| `65280` | `0x00FF00` | `#00FF00` |
| `16776960` | `0xFFFF00` | `#FFFF00` |
| `16711935` | `0xFF00FF` | `#FF00FF` |
| `-1` / 空 | 非法 | 用 `default_color`（`#FFFFFF`） |

---

## 三、本服务内部使用的资源站字段

### 3.1 分享页 `#share-boot`（HTML 内嵌 JSON）

| 参数名 | 类型 | 说明 | 示例 |
|---|---|---|---|
| `shareKey` | string | 分享标识 | `"c365c8cf..."` |
| `title` | string | 页面标题 | `"庆余年"` |
| `contentId` | string | 内容 ID | `"12345"` |
| `contentTitle` | string | **剧名**（用于去 LogVar 搜索） | `"庆余年 第一季(2019)"` |
| `unit` | string | 当前播放单元 ID | `"u1"` |
| `navigation` | array | 单元列表，见 3.2 | `[...]` |

### 3.2 `navigation[]`

| 参数名 | 类型 | 说明 | 示例 |
|---|---|---|---|
| `unitId` | string | 单元 ID，与 `unit` 比对可定位当前集 | `"u1"` |
| `label` | string | 显示名 | `"第1集"` |
| `unitKind` | string | 单元类型 | `"episode"` |
| `seasonNumber` | int | 季号 | `1` |
| `episodeNumber` | int | 集号 | `1` |

### 3.3 豆瓣兜底

| 项 | 值 |
|---|---|
| 路径 | `GET https://m.douban.com/rexxar/api/v2/movie/{douban_id}?for_mobile=1` |

| 参数名 | 类型 | 说明 | 示例 |
|---|---|---|---|
| `title` | string | 影片名 | `"庆余年"` |
| `type` | string | `movie`（电影）/ `tv`（剧集） | `"tv"` |
| `year` | string | 年份 | `"2019"` |

---

## 四、管理接口字段

### 4.1 请求体

| 参数名 | 类型 | 说明 | 示例 |
|---|---|---|---|
| `resource_hosts` | []string | 资源站主机列表 | `["jimaoys95.com","https://jimaoys94.com"]` |
| `source_priority` | []string | 选源优先级 | `["tencent","iqiyi","bilibili"]` |
| `logvar_base` | string | LogVar 根地址 | `"https://1.501710491.xyz/weixiong"` |
| `logvar_token` | string | LogVar 令牌 | `"xxx"` |

### 4.2 鉴权请求头

| 请求头 | 类型 | 说明 |
|---|---|---|
| `X-Admin-Token` | string | 等于配置中的 `admin_token`；也支持 `?token=` 查询参数 |

### 4.3 `GET /admin/sources` 响应

| 参数名 | 类型 | 说明 |
|---|---|---|
| `code` | int | `1` 成功 |
| `data.resource_hosts` | []string | 当前资源站 |
| `data.source_priority` | []string | 当前选源优先级 |
| `data.logvar_base` | string | 当前 LogVar 地址 |
| `data.logvar_token` | string | 令牌（中间打码） |
| `data.count` | int | 资源站数量 |

### 4.4 增删结果响应

| 参数名 | 类型 | 说明 |
|---|---|---|
| `added` | []string | 本次实际新增（已自动规范化、去重后） |
| `removed` | []string | 本次实际删除 |
| `resource_hosts` | []string | 操作后的全量列表 |
| `count` | int | 操作后的数量 |
*（内容由AI生成，仅供参考）*
