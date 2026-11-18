package main

// takeover 人工接管模式（D16）：以账号身份打开一个可视化（headless=false）浏览器，
// 供人工查验/操作。账号上下文全部来自环境变量（与 agent 实例契约一致）：
//   COOKIES_PATH  账号 cookies 文件（instances/<id>/cookies/cookies.json）
//   XHS_FP_SEED   账号指纹 seed
//   XHS_PROXY     出口代理（hk 账号必走，防 IP 漂移）
// 浏览器窗口关闭（Alive 探测）后进程退出；profile 为临时目录，退出清理，
// 不回写 cookies（接管会话不污染账号状态）。

import (
	"encoding/json"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/sirupsen/logrus"

	"github.com/xpzouying/xiaohongshu-mcp/configs"
	"github.com/xpzouying/xiaohongshu-mcp/cookies"
)

func runTakeover() error {
	configs.InitHeadless(false) // takeover 强制可视化（覆盖 -headless 默认）
	b := newBrowser()

	page := b.NewPage()
	if data, err := cookies.NewLoadCookie(cookies.GetCookiesFilePath()).LoadCookies(); err == nil {
		var cks []*proto.NetworkCookie
		if json.Unmarshal(data, &cks) == nil && len(cks) > 0 {
			if err := page.Browser().SetCookies(proto.CookiesToParams(cks)); err == nil {
				logrus.Infof("takeover: 已注入 %d 条账号 cookies", len(cks))
			} else {
				logrus.Warnf("takeover: cookies 注入失败（以当前会话打开）: %v", err)
			}
		}
	} else {
		logrus.Warnf("takeover: cookies 加载失败（未登录态打开）: %v", err)
	}

	if err := page.Navigate("https://www.xiaohongshu.com/explore"); err != nil {
		logrus.Warnf("takeover: 导航失败: %v（浏览器仍保持打开）", err)
	} else {
		_ = page.WaitLoad()
	}
	logrus.Info("takeover: 浏览器已打开，人工接管中（关闭窗口即退出）")

	for b.Alive() {
		time.Sleep(time.Second)
	}
	logrus.Info("takeover: 浏览器已关闭，进程退出")
	return nil
}
