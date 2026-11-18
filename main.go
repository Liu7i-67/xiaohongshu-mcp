package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/browser"
	"github.com/xpzouying/xiaohongshu-mcp/configs"
	"github.com/xpzouying/xiaohongshu-mcp/cookies"
)

// version 构建版本号，发布时通过 -ldflags "-X main.version=vX.Y.Z" 注入。
var version = "dev"

func main() {
	var (
		headless bool
		port     string
		token    string
		dataDir  string
		takeover bool
	)
	flag.BoolVar(&headless, "headless", true, "是否无头模式")
	flag.BoolVar(&takeover, "takeover", false, "人工接管模式：以账号身份打开可视化浏览器（cookies/seed/代理取环境变量），浏览器关闭即退出")
	flag.StringVar(&port, "port", ":18060", "端口")
	flag.StringVar(&token, "token", "", "鉴权 Token，留空则读取 AUTH_TOKEN")
	flag.StringVar(&dataDir, "data-dir", ".", "数据目录（实例日志 instance.log 落此目录，xhs-workbench Agent 管理模式必传）")
	flag.Parse()
	if token == "" {
		token = os.Getenv("AUTH_TOKEN")
	}

	// 实例日志落 -data-dir（xhs-workbench 契约：Agent 尾随 instance.log 上报）；
	// 同步保留 stderr，供启动器引导期抓取。必须在 EnsureBrowser 之前配置，
	// 浏览器下载进度才能进日志。
	if err := setupLogFile(dataDir); err != nil {
		logrus.WithError(err).Warn("instance.log 初始化失败，仅输出 stderr")
	} else {
		logrus.Infof("data-dir: %s", dataDir)
	}

	logrus.Infof("xiaohongshu-mcp version: %s", version)

	// 只用内置浏览器。启动时就备好，缺它直接退出，不拖到第一个请求才失败。
	binPath, err := browser.EnsureBrowser()
	if err != nil {
		logrus.Fatalf("%v", err)
	}
	logrus.Infof("using browser binary: %s", binPath)

	configs.InitHeadless(headless)
	// 入口层解析出 seed 和代理，经 configs 透传给浏览器工厂。
	// seed 取值：环境变量 > 会话文件 > 新生成并写回，保证同一账号每次启动一致。
	configs.SetFingerprintSeed(configs.ResolveFingerprintSeed(
		cookies.NewLoadCookie(cookies.GetCookiesFilePath())))
	configs.SetProxy(configs.ProxyFromEnv())
	if pt := configs.PageTimeoutFromEnv(); pt > 0 {
		configs.InitPageTimeout(pt)
		logrus.Infof("page timeout budget: %ds", pt)
	}

	// 人工接管模式（D16）：不开 HTTP 服务，单浏览器 + 账号上下文，窗口关即退出
	if takeover {
		if err := runTakeover(); err != nil {
			logrus.Fatalf("takeover 失败: %v", err)
		}
		return
	}

	// 初始化服务
	xiaohongshuService := NewXiaohongshuService()

	// 创建并启动应用服务器
	appServer := NewAppServer(xiaohongshuService, token)
	if err := appServer.Start(port); err != nil {
		logrus.Fatalf("failed to run server: %v", err)
	}
}

// setupLogFile 把 logrus 输出切到 <dataDir>/instance.log（追加）+ stderr 双写。
func setupLogFile(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "instance.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	logrus.SetOutput(io.MultiWriter(f, os.Stderr))
	return nil
}
