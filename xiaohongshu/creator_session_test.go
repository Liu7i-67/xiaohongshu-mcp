package xiaohongshu

import "testing"

func TestIsCreatorHomeURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"首页裸路径", "https://creator.xiaohongshu.com", true},
		{"首页带斜杠", "https://creator.xiaohongshu.com/", true},
		{"首页带query", "https://creator.xiaohongshu.com/?source=official", true},
		{"正式页", "https://creator.xiaohongshu.com/publish/publish?source=official", true},
		{"dashboard路径", "https://creator.xiaohongshu.com/dashboard", true},
		{"登录页排除", "https://creator.xiaohongshu.com/login?service=http", false},
		{"中转域排除", "https://customer.xiaohongshu.com/v2/login", false},
		{"主站排除", "https://www.xiaohongshu.com/explore", false},
		{"空URL", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isCreatorHomeURL(c.url); got != c.want {
				t.Fatalf("isCreatorHomeURL(%q) = %v, want %v", c.url, got, c.want)
			}
		})
	}
}
