package server

import (
	"context"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func css(selector string) chromedp.CSS {
	return chromedp.CSS(selector)
}

func enableNetwork() chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
		_, err := cdp.Call(ctx, target, network.Enable, network.EnableParams{})
		return err
	})
}

func clearBrowserCookies() chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
		_, err := cdp.Call(ctx, target, network.ClearBrowserCookies, cdp.Empty{})
		return err
	})
}

func setSessionCookie(pageURL, token string) chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
		_, err := cdp.Call(ctx, target, network.SetCookie, network.SetCookieParams{
			Name:  middleware.CookieName,
			Value: token,
			URL:   pageURL,
			Path:  "/",
		})
		return err
	})
}

func setEmulatedMediaFeatures(features []*emulation.MediaFeature) chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
		_, err := cdp.Call(ctx, target, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{
			Features: features,
		})
		return err
	})
}

// resultInto runs an action and stores its value. chromedp v0.22 actions
// return their result, and Do only sequences actions that return nothing.
func resultInto[T any](action chromedp.Action[T], dest *T) chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
		value, err := action(ctx, target)
		if err != nil {
			return err
		}
		*dest = value
		return nil
	})
}

func evalInto[T any](expression string, dest *T) chromedp.Action[chromedp.Void] {
	return resultInto(chromedp.Evaluate[T](expression), dest)
}
