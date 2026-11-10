package xiaohongshu

import (
	"context"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

// 卡片风格：文字生成图片预览页九宫格的 caption 文案，缺省"基础"。
const (
	CardStyleBasic       = "基础"
	CardStyleComic       = "美漫"
	CardStyleSketch      = "插图"
	CardStyleMemo        = "备忘"
	CardStyleGraffiti    = "涂鸦"
	CardStyleBorder      = "边框"
	CardStyleFresh       = "清新"
	CardStylePaint       = "涂写"
	CardStyleHandwritten = "手写"
)

// validCardStyles 全部支持的卡片风格。
var validCardStyles = []string{
	CardStyleBasic, CardStyleComic, CardStyleSketch, CardStyleMemo,
	CardStyleGraffiti, CardStyleBorder, CardStyleFresh, CardStylePaint,
	CardStyleHandwritten,
}

// IsValidCardStyle 风格是否受支持。
func IsValidCardStyle(style string) bool {
	for _, s := range validCardStyles {
		if s == style {
			return true
		}
	}
	return false
}

// ValidCardStyles 全部支持的卡片风格。
func ValidCardStyles() []string {
	return validCardStyles[:]
}

// 文字配图流程各入口/按钮的页面文案。
const (
	textCardEntryText   = "文字配图"
	textCardAnotherBtn  = "再写一张"
	textCardGenerateBtn = "生成图片"
	textCardNextBtn     = "下一步"
	textCardGuideBtn    = "我知道了"
)

// 卡片编辑器查找的轮询窗口
const cardEditorTimeout = 10 * time.Second

// PublishTextCardContent 发布"文字配图"笔记：每条文本生成一张卡片图作为配图
type PublishTextCardContent struct {
	Texts        []string // 每条文字一张卡片
	CardStyle    string   // 卡片风格，空 = 基础
	Title        string
	Content      string
	Tags         []string
	ScheduleTime *time.Time // 定时发布时间，nil 表示立即发布
	IsOriginal   bool       // 是否声明原创
	Visibility   string     // 可见范围: "公开可见"(默认), "仅自己可见", "仅互关好友可见"
	Products     []string   // 商品关键词列表，用于绑定带货商品
}

type PublishTextCardAction struct {
	page *rod.Page
}

// NewPublishTextCardAction 进入发布页并停留在"上传图文" TAB（文字配图入口在其中）。
func NewPublishTextCardAction(page *rod.Page) (*PublishTextCardAction, error) {
	pp := page.Timeout(300 * time.Second)

	if err := ensureCreatorSession(pp); err != nil {
		return nil, err
	}

	if err := pp.Navigate(urlOfPublic); err != nil {
		return nil, errors.Wrap(err, "导航到发布页面失败")
	}

	if err := pp.WaitLoad(); err != nil {
		logrus.Warnf("等待页面加载出现问题: %v，继续尝试", err)
	}
	time.Sleep(2 * time.Second)

	if err := pp.WaitDOMStable(time.Second, 0.1); err != nil {
		logrus.Warnf("等待 DOM 稳定出现问题: %v，继续尝试", err)
	}
	time.Sleep(1 * time.Second)

	if err := mustClickPublishTab(pp, "上传图文"); err != nil {
		logrus.Errorf("点击上传图文 TAB 失败: %v", err)
		return nil, err
	}

	time.Sleep(1 * time.Second)

	return &PublishTextCardAction{
		page: pp,
	}, nil
}

func (p *PublishTextCardAction) Publish(ctx context.Context, content PublishTextCardContent) error {
	if len(content.Texts) == 0 {
		return errors.New("文字卡片不能为空")
	}

	cardStyle := content.CardStyle
	if cardStyle == "" {
		cardStyle = CardStyleBasic
	}

	// 重设超时：.Context(ctx) 会替换掉 NewPublishTextCardAction 里 Timeout(300s) 的 deadline
	page := p.page.Context(ctx).Timeout(300 * time.Second)

	logrus.Infof("发布内容: title=%s, texts=%d张, card_style=%s, tags=%v, schedule=%v, original=%v, visibility=%s, products=%v",
		content.Title, len(content.Texts), cardStyle, content.Tags, content.ScheduleTime, content.IsOriginal, content.Visibility, content.Products)

	if err := generateTextCards(page, content.Texts, cardStyle); err != nil {
		return err
	}

	if err := submitPublish(ctx, page, content.Title, content.Content, content.Tags, content.ScheduleTime, content.IsOriginal, content.Visibility, content.Products); err != nil {
		return errors.Wrap(err, "小红书发布失败")
	}

	return nil
}

// generateTextCards 从「文字配图」入口走到图片回填发布表单：
// 写文字（可多张）→ 生成图片 → 选风格 → 下一步 → 关引导弹窗。
func generateTextCards(page *rod.Page, texts []string, cardStyle string) error {
	if err := mustClickByText(page, textCardEntryText); err != nil {
		return errors.Wrap(err, "进入文字配图失败")
	}
	time.Sleep(1 * time.Second)

	// 逐条输入；第 2 条起先点「再写一张」新增输入框，新编辑器总是排在最后
	for i, text := range texts {
		if i > 0 {
			if err := mustClickByText(page, textCardAnotherBtn); err != nil {
				return errors.Wrap(err, "点击再写一张失败")
			}
			time.Sleep(500 * time.Millisecond) // 技术等待：等新输入框挂载
		}
		if err := typeIntoCardEditor(page, i, text); err != nil {
			return err
		}
	}

	if err := mustClickByText(page, textCardGenerateBtn); err != nil {
		return errors.Wrap(err, "点击生成图片失败")
	}

	if err := waitCardStyleGrid(page); err != nil {
		return err
	}

	if err := clickCardByStyle(page, cardStyle); err != nil {
		return err
	}

	if err := mustClickByText(page, textCardNextBtn); err != nil {
		return errors.Wrap(err, "点击下一步失败")
	}

	// 回填后可能弹「图片可以编辑啦」引导，出现就关掉，不出现直接过
	closeImageEditGuide(page)
	return nil
}

// typeIntoCardEditor 把第 index 条文字输入对应的卡片编辑器（tiptap ProseMirror）。
func typeIntoCardEditor(page *rod.Page, index int, text string) error {
	deadline := time.Now().Add(cardEditorTimeout)
	for time.Now().Before(deadline) {
		elems, err := page.Elements("div.ProseMirror")
		if err == nil {
			vis := visibleElements(elems)
			if index < len(vis) {
				if err := humanize.Type(page.GetContext(), vis[index], text); err != nil {
					return errors.Wrap(err, "输入卡片文字失败")
				}
				humanize.Delay(page.GetContext(), humanize.AfterType)
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.Errorf("第 %d 个卡片编辑器没有出现（超时 %v）", index+1, cardEditorTimeout)
}

// waitCardStyleGrid 等待生成结果九宫格渲染出来（以"基础" caption 出现为准）。
func waitCardStyleGrid(page *rod.Page) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		elem, _, err := findClickableByText(page, CardStyleBasic)
		if err == nil && elem != nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("等待卡片风格列表超时（30 秒）")
}

// clickCardByStyle 在九宫格中点选指定风格的卡片缩略图。
// 风格集合由平台按次随机生成，未必每次都全：请求风格缺失时降级为
// 基础（默认预选）→ 网格里第一张有缩略图的卡片，并把可用风格记进日志。
// 点击目标是 caption 同卡片内的 img（与人工点选行为一致，caption 自身未必绑事件）。
func clickCardByStyle(page *rod.Page, style string) error {
	deadline := time.Now().Add(10 * time.Second)
	var available string
	for time.Now().Before(deadline) {
		if clickStyleOnPage(page, style) {
			logrus.Infof("已选择卡片风格: %s", style)
			time.Sleep(500 * time.Millisecond) // 技术等待：等选中态生效
			return nil
		}
		available = listCardStyles(page)
		time.Sleep(500 * time.Millisecond)
	}

	logrus.Warnf("预览页没有卡片风格 %q，可用风格: [%s]", style, available)

	if clickStyleOnPage(page, CardStyleBasic) {
		logrus.Infof("风格 %q 缺失，已降级选择: 基础", style)
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	// 基础也没有（caption 可能不在已知清单里）：点网格中第一张适中缩略图兜底
	res, err := page.Eval(`() => {
		for (const img of document.querySelectorAll('img')) {
			const r = img.getBoundingClientRect();
			if (r.width >= 60 && r.width <= 500 && r.height >= 60) { img.click(); return true; }
		}
		return false;
	}`)
	if err == nil && res.Value.Bool() {
		logrus.Info("已降级选择网格中第一张可用卡片")
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	return errors.Errorf("没有找到卡片风格 - %s（可用: [%s]）", style, available)
}

// clickStyleOnPage 单次尝试：找到风格 caption 并点击所在卡片内的缩略图。
func clickStyleOnPage(page *rod.Page, style string) bool {
	res, err := page.Eval(`(style) => {
		const caps = [...document.querySelectorAll('*')].filter(n =>
			n.childElementCount === 0 && n.textContent.trim() === style);
		for (const cap of caps) {
			const r0 = cap.getBoundingClientRect();
			if (r0.width <= 1 || r0.height <= 1) continue;
			let node = cap;
			for (let up = 0; up < 4 && node.parentElement; up++) {
				node = node.parentElement;
				const img = node.querySelector('img');
				if (img) {
					const r = img.getBoundingClientRect();
					if (r.width >= 60 && r.height >= 60) { img.click(); return true; }
				}
			}
		}
		return false;
	}`, style)
	return err == nil && res.Value.Bool()
}

// listCardStyles 列出预览页当前实际渲染出来的已知风格（caption 可见为准），"|" 拼接。
func listCardStyles(page *rod.Page) string {
	res, err := page.Eval(`(names) => {
		const out = [];
		for (const n of names) {
			const cap = [...document.querySelectorAll('*')].find(x =>
				x.childElementCount === 0 && x.textContent.trim() === n);
			if (cap) {
				const r = cap.getBoundingClientRect();
				if (r.width > 1 && r.height > 1) out.push(n);
			}
		}
		return out.join('|');
	}`, validCardStyles)
	if err != nil {
		return ""
	}
	return res.Value.String()
}

// closeImageEditGuide 「图片可以编辑啦」引导弹窗：出现就点「我知道了」，不出现直接过。
func closeImageEditGuide(page *rod.Page) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		elem, blocked, err := findClickableByText(page, textCardGuideBtn)
		if err == nil && elem != nil && !blocked {
			if err := humanize.Click(elem); err == nil {
				logrus.Info("已关闭图片编辑引导弹窗")
				time.Sleep(300 * time.Millisecond) // 技术等待：等弹窗收起
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// mustClickByText 按精确文本点击可见叶子元素，浮层遮挡时先关浮层。
// 仿 mustClickPublishTab 的轮询与降级策略。
func mustClickByText(page *rod.Page, text string) error {
	deadline := time.Now().Add(15 * time.Second)
	blockedAtLeastOnce := false

	for time.Now().Before(deadline) {
		elem, blocked, err := findClickableByText(page, text)
		if err != nil {
			logrus.Warnf("查找元素 %s 失败: %v", text, err)
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if elem == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if blocked {
			blockedAtLeastOnce = true
			logrus.Info("元素被遮挡，尝试关闭浮层")
			dismissPopCover(page)
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if err := humanize.Click(elem); err != nil {
			logrus.Warnf("点击元素 %s 失败: %v", text, err)
			time.Sleep(200 * time.Millisecond)
			continue
		}

		return nil
	}

	// 区分两种失败：找不到元素，和找到了但浮层一直关不掉
	if blockedAtLeastOnce {
		return errors.Errorf("元素 %s 一直被浮层遮挡，Esc 与点击空白都未能关闭", text)
	}
	return errors.Errorf("没有找到可点击元素 - %s", text)
}

// findClickableByText 按精确文本找可见、有实际形状且未被遮挡的叶子元素。
func findClickableByText(page *rod.Page, text string) (*rod.Element, bool, error) {
	elems, err := page.Elements("div,button,span,a,p")
	if err != nil {
		return nil, false, err
	}

	for _, elem := range elems {
		txt, err := elem.Text()
		if err != nil || strings.TrimSpace(txt) != text {
			continue
		}

		// 只点叶子节点，避免命中包着目标按钮的大容器
		nc, err := elem.Eval(`() => this.childElementCount`)
		if err != nil || nc.Value.Int() > 0 {
			continue
		}

		// rod 的 humanize.Click 要求元素有实际形状
		boxed, err := elem.Eval(`() => {
			const r = this.getBoundingClientRect();
			return r.width > 1 && r.height > 1;
		}`)
		if err != nil || !boxed.Value.Bool() {
			continue
		}

		blocked, err := isElementBlocked(elem)
		if err != nil {
			continue
		}

		return elem, blocked, nil
	}

	return nil, false, nil
}

// visibleElements 过滤出有实际形状的元素。
func visibleElements(elems []*rod.Element) []*rod.Element {
	out := make([]*rod.Element, 0, len(elems))
	for _, elem := range elems {
		boxed, err := elem.Eval(`() => {
			const r = this.getBoundingClientRect();
			return r.width > 1 && r.height > 1;
		}`)
		if err == nil && boxed.Value.Bool() {
			out = append(out, elem)
		}
	}
	return out
}
