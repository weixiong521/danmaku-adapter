package main

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestParseAnimeTitle(t *testing.T) {
	cases := []struct {
		raw        string
		wantBase   string
		wantSeason int
		wantYear   int
	}{
		{"庆余年 第一季(2019)【国产剧】from tencent", "庆余年", 1, 2019},
		{"庆余年 第二季(2024)【国产剧】from tencent", "庆余年", 2, 2024},
		{"某剧 from iqiyi", "某剧", 1, 0},
		{"狂飙(2023)【国产剧】from iqiyi", "狂飙", 1, 2023},
	}
	for _, c := range cases {
		base, season, year := parseAnimeTitle(c.raw)
		if base != c.wantBase || season != c.wantSeason || year != c.wantYear {
			t.Errorf("parseAnimeTitle(%q) = (%q,%d,%d), want (%q,%d,%d)",
				c.raw, base, season, year, c.wantBase, c.wantSeason, c.wantYear)
		}
	}
}

func TestColor(t *testing.T) {
	if got := decColorToHex("16777215", "#FFFFFF"); got != "#FFFFFF" {
		t.Errorf("white = %q", got)
	}
	if got := decColorToHex("16718180", "#000000"); got != "#FF1964" {
		t.Errorf("16718180 = %q, want #FF1964", got)
	}
	if got := decColorToHex("abc", "#FFFFFF"); got != "#FFFFFF" {
		t.Errorf("invalid = %q", got)
	}
	if got := decColorToHex("999999999", "#FFFFFF"); got != "#FFFFFF" {
		t.Errorf("out-of-range = %q", got)
	}
}

func TestMode(t *testing.T) {
	want := map[int]string{1: "right", 2: "top", 3: "bottom", 4: "left", 99: "right"}
	for mode, pos := range want {
		if got := modeToPosition(mode); got != pos {
			t.Errorf("mode %d = %q, want %q", mode, got, pos)
		}
	}
}

func TestSampleAndBuild(t *testing.T) {
	comments := make([]Comment, 100)
	for i := range comments {
		comments[i] = Comment{P: "1.00,1,16777215,[qq]", M: "x"}
	}
	cfg := &Config{MaxDanmu: 10, FontSize: "24px", DefaultColor: "#FFFFFF"}
	out := buildDanmuku(comments, cfg, 100)
	if len(out) != 10 {
		t.Fatalf("capped entries = %d, want 10", len(out))
	}
	// 抽样应覆盖到靠后的时间轴：最后一条应来自接近末尾的源弹幕
	last := out[len(out)-1]
	if last[0] != "1.00" { // 所有时间相同，仅验证结构
		t.Errorf("unexpected time %v", last[0])
	}
}

func TestPickEpisode(t *testing.T) {
	eps := make([]Episode, 46)
	for i := range eps {
		n := i + 1
		eps[i] = Episode{
			EpisodeID:     json.Number(strconv.Itoa(n + 1000)),
			EpisodeNumber: strconv.Itoa(n),
		}
	}
	if id := pickEpisode(eps, 1); id != "1001" {
		t.Errorf("ep1 = %q, want 1001", id)
	}
	if id := pickEpisode(eps, 46); id != "1046" {
		t.Errorf("ep46 = %q, want 1046", id)
	}
	if id := pickEpisode(eps, 100); id != "1046" {
		t.Errorf("clamp = %q, want 1046", id)
	}
	if id := pickEpisode(eps, 0); id != "1001" {
		t.Errorf("default = %q, want 1001", id)
	}
}
