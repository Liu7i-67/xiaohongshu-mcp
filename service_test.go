package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// timeNowPlus 生成以 RFC3339 表示的 n 之后的时刻。
func timeNowPlus(d time.Duration) string {
	return time.Now().Add(d).Format(time.RFC3339)
}

// PublishContent 的校验都在进浏览器之前返回，可以纯单测覆盖；
// 校验通过的路径会真的起浏览器发布，不在单测范围（由 E2E 验证）。
func TestPublishContentTextCardValidation(t *testing.T) {
	s := NewXiaohongshuService()
	ctx := context.Background()

	base := func(mutate func(*PublishRequest)) *PublishRequest {
		req := &PublishRequest{
			Title:      "发帖测试：新账号首篇",
			Content:    "这是一篇发帖链路测试笔记，仅自己可见。",
			Visibility: "仅自己可见",
		}
		mutate(req)
		return req
	}

	longText := strings.Repeat("雨", maxTextCardChars+1)

	tests := []struct {
		name    string
		req     *PublishRequest
		wantErr string
	}{
		{
			name:    "images 和 text_cards 都不传",
			req:     base(func(r *PublishRequest) {}),
			wantErr: "images 和 text_cards 至少提供一个",
		},
		{
			name: "text_cards 与 images 同时提供",
			req: base(func(r *PublishRequest) {
				r.Images = []string{"C:/x.png"}
				r.TextCards = []string{"成都为什么还不下雨"}
			}),
			wantErr: "text_cards 与 images 不能同时提供",
		},
		{
			name: "text_cards 条数超限",
			req: base(func(r *PublishRequest) {
				for i := 0; i <= maxTextCards; i++ {
					r.TextCards = append(r.TextCards, "成都为什么还不下雨")
				}
			}),
			wantErr: "条数须在",
		},
		{
			name: "text_cards 单条超长",
			req: base(func(r *PublishRequest) {
				r.TextCards = []string{longText}
			}),
			wantErr: "超过 200 字限制",
		},
		{
			name: "text_cards 含空白条目",
			req: base(func(r *PublishRequest) {
				r.TextCards = []string{"成都为什么还不下雨", "   "}
			}),
			wantErr: "第 2 条为空",
		},
		{
			name: "非法卡片风格",
			req: base(func(r *PublishRequest) {
				r.TextCards = []string{"成都为什么还不下雨"}
				r.CardStyle = "赛博朋克"
			}),
			wantErr: "不支持的卡片风格",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.PublishContent(ctx, tt.req)
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseScheduleAt(t *testing.T) {
	// 空串 = 立即发布
	got, err := parseScheduleAt("")
	assert.NoError(t, err)
	assert.Nil(t, got)

	// 格式错误
	_, err = parseScheduleAt("2026-11-10 10:00:00")
	assert.ErrorContains(t, err, "ISO8601")

	// 30 分钟后：早于 1 小时下限
	_, err = parseScheduleAt(timeNowPlus(30 * time.Minute))
	assert.ErrorContains(t, err, "至少在1小时后")

	// 15 天后：超过 14 天上限
	_, err = parseScheduleAt(timeNowPlus(15 * 24 * time.Hour))
	assert.ErrorContains(t, err, "不能超过14天")
}
