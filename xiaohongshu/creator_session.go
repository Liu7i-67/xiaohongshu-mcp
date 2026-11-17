package xiaohongshu

import (
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

const (
	urlOfCreatorHome = `https://creator.xiaohongshu.com/?source=official`

	creatorSessionCookie = "galaxy_creator_session_id"
	creatorHomePrefix    = "https://creator.xiaohongshu.com"

	// creatorSessionWait 预热总窗口：SSO 重定向链（creator → customer → creator）
	// 在新实例/冷 profile 上可能超过旧逻辑的 15s，导致发布全量失败
	//（2026-11-17 .212 实锤：扫码登录后 16s 的 POST 因等不到 cookie 而失败）。
	creatorSessionWait = 45 * time.Second
	// creatorSessionReNav 单次导航的窗口，超过则重新导航触发 SSO 链（防链路卡死）。
	creatorSessionReNav = 15 * time.Second
)

// ensureCreatorSession 确保浏览器持有创作者中心会话（发帖/视频/文字配图共用）。
//
// 会话建立判定（任一命中即成功）：
//  1. 浏览器 cookie 出现 galaxy_creator_session_id（SSO 链完成的标准信号）；
//  2. 页面态兜底：落在创作者中心正式页（非 /login）——cookie 换名时仍能放行，
//     若会话真失效，后续发布页会在更具体的步骤报错。
//
// 全程日志 + 失败带最终 URL，便于设备端排障。
func ensureCreatorSession(page *rod.Page) error {
	if hasCreatorSession(page) {
		return nil
	}

	deadline := time.Now().Add(creatorSessionWait)
	nextNav := time.Time{}
	lastURL := ""
	for time.Now().Before(deadline) {
		if nextNav.IsZero() || time.Now().After(nextNav) {
			if err := page.Navigate(urlOfCreatorHome); err != nil {
				return errors.Wrap(err, "打开创作者中心失败")
			}
			logrus.Infof("创作者中心预热: 导航 %s（剩余 %v）",
				urlOfCreatorHome, time.Until(deadline).Round(time.Second))
			nextNav = time.Now().Add(creatorSessionReNav)
		}

		time.Sleep(500 * time.Millisecond)

		if hasCreatorSession(page) {
			humanize.Delay(page.GetContext(), humanize.AfterNavigate)
			logrus.Info("创作者中心预热: 会话 cookie 已建立")
			return nil
		}

		if info, err := page.Info(); err == nil && info != nil {
			lastURL = info.URL
			if isCreatorHomeURL(lastURL) {
				humanize.Delay(page.GetContext(), humanize.AfterNavigate)
				logrus.Infof("创作者中心预热: 页面态兜底命中 URL=%s", lastURL)
				return nil
			}
		}
	}
	return errors.Errorf("创作者中心登录失效，请重新扫码登录（预热 %v 未建立会话，最终 URL=%s）",
		creatorSessionWait, lastURL)
}

func hasCreatorSession(page *rod.Page) bool {
	cks, err := page.Browser().GetCookies()
	if err != nil {
		return false
	}
	for _, c := range cks {
		if c.Name == creatorSessionCookie {
			return true
		}
	}
	return false
}

// isCreatorHomeURL 页面态兜底判定：URL 在创作者中心域且不是登录页。
// SSO 中转域（customer.xiaohongshu.com 等）不命中；/login 路径不命中。
func isCreatorHomeURL(u string) bool {
	rest, ok := strings.CutPrefix(u, creatorHomePrefix)
	if !ok {
		return false
	}
	if rest == "" || rest == "/" || strings.HasPrefix(rest, "/?") {
		return true
	}
	return !strings.Contains(strings.ToLower(rest), "login")
}
