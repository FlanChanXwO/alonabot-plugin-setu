// Package plugin 响应聊天中的普通插画请求。
package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/AlonaBot/AlonaBot/sdk"
	"github.com/AlonaBot/AlonaBot/sdk/media"
	"github.com/AlonaBot/AlonaBot/sdk/message"
)

const (
	trigger      = `^来(-?[0-9]+|[零〇一二两三四五六七八九十]+)?份色图$`
	failureReply = "这次没能发出色图，请稍后再试。"
)

type config struct {
	forwardMode string
	maxPictures int
}

func (settings config) useForward(adapterID string, count int) bool {
	return adapterID == "onebot11" &&
		(settings.forwardMode == "always" || settings.forwardMode == "multi" && count > 1)
}

// New 注册色图正则路由，并为插件实例创建共享下载队列。
func New() *sdk.Plugin {
	return newPlugin(newPictureSource())
}

func newPlugin(source *pictureSource) *sdk.Plugin {
	plugin := sdk.NewPlugin()
	plugin.Regex(trigger).ID("setu").Permission(sdk.Anyone).Handle(func(ctx *sdk.Context) error {
		values := ctx.Config()
		settings := config{
			forwardMode: values.String("forward_mode"),
			maxPictures: int(values.Int("max_pictures")),
		}
		count := parseCount(ctx.Match()[1])
		if count < 1 || count > settings.maxPictures {
			return ctx.Reply(fmt.Sprintf("一次可以请求 1 到 %d 张色图。", settings.maxPictures))
		}
		if err := sendPictures(ctx, source, settings, count); err != nil {
			slog.WarnContext(ctx, "Setu request failed", "count", count, "error", err)
			if ctx.Err() != nil {
				return err
			}
			if replyErr := ctx.Reply(failureReply); replyErr != nil {
				return errors.Join(err, fmt.Errorf("send failure notice: %w", replyErr))
			}
		}
		return nil
	})
	return plugin
}

func parseCount(text string) int {
	switch text {
	case "":
		return 1
	case "两":
		return 2
	case "十":
		return 10
	}
	for index, symbol := range []rune("零一二三四五六七八九") {
		if text == string(symbol) {
			return index
		}
	}
	count, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return count
}

func sendPictures(ctx *sdk.Context, source *pictureSource, settings config, count int) error {
	pictures, err := source.collect(ctx, count)
	if err != nil {
		return fmt.Errorf("collect pictures: %w", err)
	}
	chain := make(message.Chain, 0, count)
	for _, picture := range pictures {
		// 每张原图交给 Core 管理，避免多个内联片段合计超过 4 MiB 的 SDK 帧限制。
		ref, err := media.Upload(ctx, bytes.NewReader(picture.content), picture.mime)
		if err != nil {
			return fmt.Errorf("upload picture: %w", err)
		}
		chain = append(chain, message.Image(ref.Source()))
	}
	var content message.Content = chain
	adapterID, _, _, err := message.ParseSID(ctx.Session())
	if err != nil {
		return fmt.Errorf("resolve destination adapter: %w", err)
	}
	if settings.useForward(adapterID, count) {
		items := make([]message.ForwardItem, 0, count)
		for _, segment := range chain {
			items = append(items, message.ForwardItem{Chain: message.Chain{segment}})
		}
		content = message.Chain{message.ForwardSegment{Items: items}}
	}
	if _, err := ctx.Send(content); err != nil {
		return fmt.Errorf("send pictures: %w", err)
	}
	return nil
}
