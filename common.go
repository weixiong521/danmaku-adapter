package main

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reFromSuffix = regexp.MustCompile(`(?i)\s*from\s+\S+\s*$`)
	reBracket    = regexp.MustCompile(`【[^】]*】|\[[^\]]*\]`)
	reYearParen  = regexp.MustCompile(`[（(]((?:19|20)\d{2})[）)]`)
	reSeasonCN   = regexp.MustCompile(`第\s*([0-9一二三四五六七八九十百零两]+)\s*[季期部]`)
	reSeasonEN   = regexp.MustCompile(`(?i)\b(?:season|part)\s*(\d{1,2})\b`)
	reSxx        = regexp.MustCompile(`(?i)\bS(\d{1,2})\b`)
	reLeadingNum = regexp.MustCompile(`\d+`)
)

var cnDigits = map[rune]int{
	'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
	'五': 5, '六': 6, '七': 7, '八': 8, '九': 9, '十': 10, '百': 100,
}

// parseCNNumber 解析简单中文数字（支持 1..99 以及“十”“二十”“二十三”等），失败返回 0。
func parseCNNumber(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	total := 0
	hasUnit := false
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		v, ok := cnDigits[runes[i]]
		if !ok {
			return 0
		}
		if v == 10 || v == 100 {
			hasUnit = true
			if total == 0 {
				total = 1
			}
			total *= v
			if v == 100 && i+1 < len(runes) {
				if next, ok := cnDigits[runes[i+1]]; ok && next < 10 {
					total += next
					i++
				}
			}
		} else {
			if hasUnit {
				total += v
			} else {
				total = total*10 + v
			}
		}
	}
	return total
}

// parseAnimeTitle 从 LogVar 的 animeTitle 中解析出基础剧名、季号、年份。
// 例：“庆余年 第一季(2019)【国产剧】from tencent” -> “庆余年”, 1, 2019
func parseAnimeTitle(raw string) (base string, season int, year int) {
	t := reFromSuffix.ReplaceAllString(raw, "")
	t = reBracket.ReplaceAllString(t, "")
	if m := reYearParen.FindStringSubmatch(t); m != nil {
		year, _ = strconv.Atoi(m[1])
		t = strings.Replace(t, m[0], " ", 1)
	}
	season = 1
	if m := reSeasonCN.FindStringSubmatch(t); m != nil {
		if n := parseCNNumber(m[1]); n > 0 {
			season = n
		}
		t = strings.Replace(t, m[0], " ", 1)
	} else if m := reSeasonEN.FindStringSubmatch(t); m != nil {
		season, _ = strconv.Atoi(m[1])
		t = strings.Replace(t, m[0], " ", 1)
	} else if m := reSxx.FindStringSubmatch(t); m != nil {
		season, _ = strconv.Atoi(m[1])
		t = strings.Replace(t, m[0], " ", 1)
	}
	base = strings.Join(strings.Fields(t), " ")
	base = strings.TrimSpace(base)
	return base, season, year
}

// normTitle 归一化标题用于比较：转小写、去除空白与常见标点。
func normTitle(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '-', '_', '.', '·', ':', '：', '/', '\\', '（', '）', '(', ')', '【', '】', '[', ']', '《', '》', '"', '\'', '!', '！', '?', '？', '、', ',', '，':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func titleEqual(a, b string) bool {
	na, nb := normTitle(a), normTitle(b)
	return na != "" && na == nb
}

// titleSimilar 宽松匹配：归一化后任一包含另一。
func titleSimilar(a, b string) bool {
	na, nb := normTitle(a), normTitle(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	return strings.Contains(na, nb) || strings.Contains(nb, na)
}

// parseLeadingInt 从字符串中提取第一个整数，失败返回 0。
func parseLeadingInt(s string) int {
	m := reLeadingNum.FindString(s)
	if m == "" {
		return 0
	}
	n, _ := strconv.Atoi(m)
	return n
}
