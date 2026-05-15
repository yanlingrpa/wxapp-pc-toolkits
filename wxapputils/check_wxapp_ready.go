package wxapputils

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"yanlingrpa.com/yanling/protocol/basic"
	"yanlingrpa.com/yanling/protocol/script"
)

type PopupDialogInfo struct {
	Border   basic.Rect `json:"border"`
	CloseBtn basic.Rect `json:"close_btn"`
}

type AppReadyData struct {
	AppName string `json:"app_name"`
	GuId    string `json:"guid"`
}

const (
	popupDetectInstruction = "Detect all modal popups that block interaction in the WeChat mini program window. Return each popup border and its close button area. Return an empty array if no popup exists."
	popupDetectSchema      = `[
  {
    "border": {"x": 0, "y": 0, "width": 0, "height": 0},
    "close_btn": {"x": 0, "y": 0, "width": 0, "height": 0}
  }
]`
	maxPopupCheckRounds = 3
	popupClickDelay     = 150 * time.Millisecond
	popupRoundDelay     = 300 * time.Millisecond
)

type popupDialogEnvelope struct {
	Popups []PopupDialogInfo `json:"popups"`
}

func clipForLog(s string, limit int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

func parsePopupDialogInfo(jsonRet string) ([]PopupDialogInfo, error) {
	// Vision.Detect usually returns a JSON array. Some models may return
	// a wrapped object (for example: {"popups": [...]}) or null/empty text.
	// This parser accepts these common forms to make runtime behavior stable.
	trimmed := strings.TrimSpace(jsonRet)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var popups []PopupDialogInfo
	if err := json.Unmarshal([]byte(trimmed), &popups); err == nil {
		return popups, nil
	}

	var envelope popupDialogEnvelope
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil {
		return envelope.Popups, nil
	}

	return nil, fmt.Errorf("unexpected detect result: %s", trimmed)
}

/**
 * 检测微信小程序是否准备就绪
 * 1. 获取GUI窗口和Body定位器
 * 2. 循环进行视觉检测，查找是否存在阻塞交互的弹窗
 * 3. 如果检测到弹窗，点击其关闭按钮并等待一段时间后继续检测，直到达到最大检测轮数
 * 4. 发布app_ready事件，通知调用方微信小程序已就绪
 */
func CheckWxappReady(rt script.ModuleRuntime, guiId string) (bool, error) {
	logger := rt.Logger()
	logger.Debug("CheckWxappReady start: guiId=%s", guiId)
	// Step 1: get gui window id from variable.
	if guiId == "" {
		logger.Error("CheckWxappReady failed: guiId not set")
		return false, fmt.Errorf("guiId not set")
	}

	// Step 2: resolve window + body locator that will be used for screenshot/click.
	win, exist := rt.OsGuiWindow(guiId)
	if !exist {
		logger.Error("CheckWxappReady failed: gui window not found, guiId=%s", guiId)
		return false, fmt.Errorf("gui windows {%s} not exist", guiId)
	}
	body, err := win.BodyLocator()
	if err != nil {
		logger.Error("CheckWxappReady failed: BodyLocator error, guiId=%s, err=%v", guiId, err)
		return false, fmt.Errorf("failed to locate body for gui window {%s}: %v", guiId, err)
	}

	// Step 3: run a few rounds of detect -> close -> re-detect.
	// If any popup is closed in this call, return false to ask caller retry once more,
	// so readiness is only true on a stable frame without blocking popups.
	popupClosed := false
	for i := 0; i < maxPopupCheckRounds; i++ {
		logger.Debug("CheckWxappReady round=%d/%d guiId=%s", i+1, maxPopupCheckRounds, guiId)

		bts, err := body.Snapshot(true)
		if err != nil {
			logger.Error("CheckWxappReady failed: snapshot error, guiId=%s, err=%v", guiId, err)
			return false, fmt.Errorf("failed to snapshot body for gui window {%s}: %v", guiId, err)
		}
		logger.Debug("CheckWxappReady snapshot captured: guiId=%s bytes=%d", guiId, len(bts))

		jsonRet, err := rt.VisionWorker().Detect(bts, popupDetectInstruction, popupDetectSchema)
		if err != nil {
			logger.Error("CheckWxappReady failed: vision detect error, guiId=%s, err=%v", guiId, err)
			return false, fmt.Errorf("failed to detect vision for gui window {%s}: %v", guiId, err)
		}
		logger.Debug("CheckWxappReady detect result preview: guiId=%s result=%s", guiId, clipForLog(jsonRet, 240))

		popups, err := parsePopupDialogInfo(jsonRet)
		if err != nil {
			logger.Error("CheckWxappReady failed: detect result parse error, guiId=%s, err=%v", guiId, err)
			return false, fmt.Errorf("failed to parse popup detect result for gui window {%s}: %v", guiId, err)
		}
		logger.Info("CheckWxappReady popup count: guiId=%s round=%d count=%d", guiId, i+1, len(popups))

		if len(popups) == 0 {
			logger.Debug("CheckWxappReady no popup found: guiId=%s round=%d", guiId, i+1)
			break
		}

		popupClosed = true
		for idx, popup := range popups {
			if popup.CloseBtn.Width <= 0 || popup.CloseBtn.Height <= 0 {
				logger.Warn("CheckWxappReady skip invalid close button rect: guiId=%s round=%d index=%d rect=%+v", guiId, i+1, idx, popup.CloseBtn)
				continue
			}

			pnt := popup.CloseBtn.CenterPoint()
			logger.Debug("CheckWxappReady click popup close: guiId=%s round=%d index=%d point=(%d,%d)", guiId, i+1, idx, pnt.X, pnt.Y)
			if err := body.Click(&pnt); err != nil {
				logger.Error("CheckWxappReady failed: click close button error, guiId=%s round=%d index=%d err=%v", guiId, i+1, idx, err)
				return false, fmt.Errorf("failed to click popup close button for gui window {%s}: %v", guiId, err)
			}
			time.Sleep(popupClickDelay)
		}

		time.Sleep(popupRoundDelay)
	}

	if popupClosed {
		logger.Info("CheckWxappReady pending: popups were closed this round, wait next check, guiId=%s", guiId)
		return false, nil
	}

	// Step 4: no blocking popup detected, publish readiness event.

	// app_ready：微信小程序已就绪，可以开始后续的自动化操作了。
	if err := rt.Publish("app_ready", AppReadyData{
		AppName: win.GetWindowTitle(),
		GuId:    guiId,
	}); err != nil {
		logger.Error("CheckWxappReady failed: publish app_ready error, guiId=%s, err=%v", guiId, err)
		return false, fmt.Errorf("failed to publish app_ready for gui window {%s}: %v", guiId, err)
	}

	logger.Info("CheckWxappReady success: app is ready, guiId=%s title=%s", guiId, win.GetWindowTitle())

	return true, nil
}
