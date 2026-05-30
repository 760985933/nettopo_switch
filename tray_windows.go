//go:build windows

package main

import (
	"time"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) initPlatformTray() {
	go systray.Run(func() {
		setupWindowsTray(a)
	}, nil)
}

func setupWindowsTray(a *App) {
	systray.SetTitle("Codex Switch")
	systray.SetTooltip("Codex Switch")

	mOpen := systray.AddMenuItem("打开面板", "显示主窗口")
	mQuit := systray.AddMenuItem("退出", "退出应用程序")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				runtime.WindowShow(a.ctx)
				runtime.WindowUnminimise(a.ctx)
			case <-mQuit.ClickedCh:
				forceQuit.Store(true)
				systray.Quit()
				runtime.Quit(a.ctx)
			}
		}
	}()

	// Initial balance fetch + periodic updates
	time.Sleep(2 * time.Second)
	_ = a.GetUsageBalance("")

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		_ = a.GetUsageBalance("")
	}
}

func (a *App) onBalanceUpdate(balance UsageBalance) {
	if balance.Error == "" && balance.AvailableBalance != "" {
		systray.SetTooltip("可用余额: " + balance.AvailableBalance + " " + balance.Currency)
	}
}
