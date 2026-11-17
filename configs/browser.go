package configs

import (
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	useHeadless = true

	fingerprintSeed = 0

	proxy = ""
)

func InitHeadless(h bool) {
	useHeadless = h
}

// IsHeadless 是否无头模式。
func IsHeadless() bool {
	return useHeadless
}

func SetFingerprintSeed(s int) {
	fingerprintSeed = s
}

func FingerprintSeed() int {
	return fingerprintSeed
}

// FingerprintSeedFromEnv 从 XHS_FP_SEED 环境变量解析固定 seed。
// 未设或非法返回 0（回退随机）。env 读取集中在配置层，浏览器工厂只收 Option。
func FingerprintSeedFromEnv() int {
	s := os.Getenv("XHS_FP_SEED")
	if s == "" {
		return 0
	}
	seed, err := strconv.Atoi(s)
	if err != nil || seed <= 0 {
		logrus.Warnf("invalid XHS_FP_SEED=%q, ignored (fallback to random seed)", s)
		return 0
	}
	return seed
}

func SetProxy(p string) {
	proxy = p
}

func Proxy() string {
	return proxy
}

// ProxyFromEnv 从 XHS_PROXY 环境变量读取代理地址。env 读取集中在配置层。
func ProxyFromEnv() string {
	return os.Getenv("XHS_PROXY")
}

// pageTimeoutSec 页面级操作预算（导航/渲染等待等单步上限秒数）。直连默认 60s 足够；
// 走代理时单请求延迟 2-25s，60s 会大面积打满（2026-11-17 .212 实锤 feeds/search
// 内部 deadline 超时）——经 XHS_PAGE_TIMEOUT_SEC 提高。
var pageTimeoutSec = 60

// InitPageTimeout 设置页面预算（<=0 忽略，保持当前值）。
func InitPageTimeout(sec int) {
	if sec > 0 {
		pageTimeoutSec = sec
	}
}

// PageTimeout 当前页面预算（duration），页面上下文统一取用。
func PageTimeout() time.Duration {
	return time.Duration(pageTimeoutSec) * time.Second
}

// PageTimeoutFromEnv 从 XHS_PAGE_TIMEOUT_SEC 读取页面预算秒数。未设/非法返回 0（用默认）。
func PageTimeoutFromEnv() int {
	s := os.Getenv("XHS_PAGE_TIMEOUT_SEC")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		logrus.Warnf("invalid XHS_PAGE_TIMEOUT_SEC=%q, ignored (fallback to default %ds)", s, pageTimeoutSec)
		return 0
	}
	return n
}
