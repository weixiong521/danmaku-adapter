package main

import (
	"testing"
)

func TestNormalizeCMSBase(t *testing.T) {
	cases := map[string]string{
		"https://www.example.com/":     "https://www.example.com",
		"http://WWW.Example.COM":       "http://www.example.com",
		"www.example.com":              "https://www.example.com",
		"https://www.example.com/sub/": "https://www.example.com/sub",
		"":                             "",
	}
	for in, want := range cases {
		if got := normalizeCMSBase(in); got != want {
			t.Errorf("normalizeCMSBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractCMSID(t *testing.T) {
	cases := map[string]string{
		"2421": "2421",
		"https://www.example.com/index.php/vod/detail/id/2421.html":           "2421",
		"https://www.example.com/index.php/vod/play/id/2421/sid/1/nid/3.html": "2421",
		"https://www.example.com/index.php/vod/detail?id=2421":                "2421",
	}
	for in, want := range cases {
		if got := extractCMSID(in); got != want {
			t.Errorf("extractCMSID(%q) = %q, want %q", in, got, want)
		}
	}
	if got := extractCMSID("no-id-here"); got != "" {
		t.Errorf("extractCMSID(no id) = %q, want empty", got)
	}
}

func TestParseCMSSources(t *testing.T) {
	html := `
	<div class="playlist">
	  <a href="/index.php/vod/play/id/2421/sid/1/nid/1.html">第 2 季 第 1 集</a>
	  <a href="/index.php/vod/play/id/2421/sid/1/nid/2.html">第 2 季 第 2 集</a>
	  <a href="/index.php/vod/play/id/2421/sid/1/nid/3.html">第 2 季 第 3 集</a>
	  <a href="/index.php/vod/play/id/2421/sid/1/nid/1.html">第 2 季 第 1 集</a>
	</div>`
	srcs := parseCMSSources(html, nil)
	if len(srcs) != 1 {
		t.Fatalf("sources = %d, want 1", len(srcs))
	}
	if srcs[0].SID != 1 {
		t.Errorf("sid = %d, want 1", srcs[0].SID)
	}
	if len(srcs[0].Episodes) != 3 {
		t.Fatalf("episodes = %d, want 3（去重后）", len(srcs[0].Episodes))
	}
	if srcs[0].Episodes[0].NID != 1 || srcs[0].Episodes[2].NID != 3 {
		t.Errorf("nid order wrong: %+v", srcs[0].Episodes)
	}
	if srcs[0].Episodes[1].Name != "第 2 季 第 2 集" {
		t.Errorf("episode name = %q", srcs[0].Episodes[1].Name)
	}
}

func TestParseCMSPlayerListAndTitle(t *testing.T) {
	html := `<script>var MacPlayerConfig={"player_list":{"jlm3u8":{"show":"巨量资源","from":"jlm3u8"}}};</script>`
	players := parseCMSPlayerList(html)
	if len(players) != 1 || players["jlm3u8"].Show != "巨量资源" {
		t.Fatalf("player_list parsed wrong: %+v", players)
	}

	title := cleanCMSTitle(`<title>某某剧 第二季 - 某某影视</title>`)
	if title != "某某剧 第二季" {
		t.Errorf("title = %q", title)
	}
}

func TestParsePlayerAAAA(t *testing.T) {
	html := `<script type="text/javascript">var player_aaaa={"flag":"jlm3u8","encrypt":0,` +
		`"url":"https://cdn.example.com/public/playback/0123456789abcdef0123456789abcdef/smart.m3u8",` +
		`"url_next":"","from":"jlm3u8","server":"","id":"2421","sid":1,"nid":2,"link_next":""};</script>`
	pa, err := parsePlayerAAAA(html)
	if err != nil {
		t.Fatalf("parsePlayerAAAA: %v", err)
	}
	if pa.Encrypt != 0 || pa.SID != 1 || pa.NID != 2 {
		t.Errorf("parsed = encrypt:%d sid:%d nid:%d", int(pa.Encrypt), int(pa.SID), int(pa.NID))
	}
	if pa.URL == "" {
		t.Error("url empty")
	}

	// 嵌套对象不应被非贪婪截断
	nested := `var player_aaaa = {"flag":"x","encrypt":1,"url":"abc","ext":{"a":{"b":1}}};`
	pa2, err := parsePlayerAAAA(nested)
	if err != nil {
		t.Fatalf("nested parsePlayerAAAA: %v", err)
	}
	if pa2.URL != "abc" || pa2.Encrypt != 1 {
		t.Errorf("nested parsed = %+v", pa2)
	}

	// 字符串中出现的花括号不应干扰配平
	brace := `var player_aaaa = {"flag":"x","encrypt":0,"url":"https://x.com/{}a.m3u8"};`
	pa3, err := parsePlayerAAAA(brace)
	if err != nil {
		t.Fatalf("brace parsePlayerAAAA: %v", err)
	}
	if pa3.URL != "https://x.com/{}a.m3u8" {
		t.Errorf("brace url = %q", pa3.URL)
	}

	if _, err := parsePlayerAAAA("<html>no script</html>"); err == nil {
		t.Error("expected error when player_aaaa missing")
	}
}

func TestCleanCMSTitle(t *testing.T) {
	// 优先取 <h1>，即便 <title> 里塞满分类与站点后缀
	withH1 := `<html><head><title>灵猪降妖_电影__视频详情-其他 - 某站</title></head>` +
		`<body><h1 class="title">灵猪降妖</h1></body></html>`
	if got := cleanCMSTitle(withH1); got != "灵猪降妖" {
		t.Errorf("h1 优先失败：got %q", got)
	}

	// 无 <h1> 时从 <title> 依次剥离站点后缀与分类尾巴
	noH1 := `<title>灵猪降妖_电影__视频详情-其他 - 免费短剧电影电视剧 - 最新全集在线观看</title>`
	if got := cleanCMSTitle(noH1); got != "灵猪降妖" {
		t.Errorf("title 兜底清洗失败：got %q", got)
	}

	// 常规 " - " 后缀
	if got := cleanCMSTitle(`<title>某某剧 第二季 - 某某影视</title>`); got != "某某剧 第二季" {
		t.Errorf("常规标题清洗失败：got %q", got)
	}

	// 无标题时返回空串
	if got := cleanCMSTitle("<html><body>无标题</body></html>"); got != "" {
		t.Errorf("无 title 应返回空串，got %q", got)
	}
}

func TestExtractBalancedObject(t *testing.T) {
	s := `xx{"a":{"b":"}"},"c":1}yy`
	want := `{"a":{"b":"}"},"c":1}`
	if got, ok := extractBalancedObject(s, 2); !ok || got != want {
		t.Errorf("extractBalancedObject = %q,%v, want %q", got, ok, want)
	}
	if _, ok := extractBalancedObject("abc", 0); ok {
		t.Error("should fail when start is not '{'")
	}
	if _, ok := extractBalancedObject(`{"a":1`, 0); ok {
		t.Error("should fail on unbalanced input")
	}
}

func TestCMSDecryptHook(t *testing.T) {
	if _, err := cmsDecryptHook("https%3A%2F%2Fx.com%2Fa.m3u8"); err != nil {
		t.Errorf("url-encoded should decode: %v", err)
	}
	if _, err := cmsDecryptHook("aHR0cHM6Ly94LmNvbS9hLm0zdTg="); err != nil {
		t.Errorf("base64 should decode: %v", err)
	}
	if _, err := cmsDecryptHook("zzzz-not-decryptable"); err == nil {
		t.Error("expected error for opaque ciphertext")
	}
}
