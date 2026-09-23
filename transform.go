package main

import (
	"fmt"
	"strconv"
	"strings"
)

// modeToPosition 弹弹play 弹幕模式 -> Getapp 位置字符串。
// 1=滚动(右出) 2=顶部 3=底部 4=反向(左出)
func modeToPosition(mode int) string {
	switch mode {
	case 2:
		return "top"
	case 3:
		return "bottom"
	case 4:
		return "left"
	case 1:
		return "right"
	default:
		return "right"
	}
}

// decColorToHex 十进制颜色 -> #RRGGBB；非法时返回兜底色。
func decColorToHex(s, fallback string) string {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 || n > 0xFFFFFF {
		return fallback
	}
	return fmt.Sprintf("#%06X", n)
}

// sampleComments 按时间轴均匀抽样到 n 条，保证覆盖整片。
func sampleComments(in []Comment, n int) []Comment {
	if n >= len(in) || n <= 0 {
		return in
	}
	out := make([]Comment, 0, n)
	for i := 0; i < n; i++ {
		idx := i * len(in) / n
		out = append(out, in[idx])
	}
	return out
}

// buildDanmuku 把 LogVar 弹幕转换为 Getapp 的 danmuku 数组。
// 单条结构：[时间, 位置, 颜色, "0", 内容, IP, 发送时间戳, 字号]
func buildDanmuku(comments []Comment, cfg *Config, now int64) [][]any {
	if cfg.MaxDanmu > 0 && len(comments) > cfg.MaxDanmu {
		comments = sampleComments(comments, cfg.MaxDanmu)
	}
	out := make([][]any, 0, len(comments))
	for _, c := range comments {
		parts := strings.Split(c.P, ",")
		if len(parts) < 3 {
			continue
		}
		timeStr := strings.TrimSpace(parts[0])
		mode := parseLeadingInt(parts[1])
		color := decColorToHex(parts[2], cfg.DefaultColor)
		entry := []any{
			timeStr,
			modeToPosition(mode),
			color,
			"0",
			c.M,
			"127.0.0.1",
			strconv.FormatInt(now, 10),
			cfg.FontSize,
		}
		out = append(out, entry)
	}
	return out
}
