package yscript

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"yanlingrpa.com/yanling/protocol/basic"
	"yanlingrpa.com/yanling/protocol/script"
)

// File: wxapputils\change_gps_location.go
type PreferedLocation struct {
	GuiId	string	`json:"gui_id"`
	Keyword	string	`json:"keyword"`
}

const (
	gpsElementDescription	= "GPS location or position button, address entry field, or location icon in the WeChat mini program"
	gpsAddressDescription	= "address selection list or location picker showing candidate addresses"
	gpsWaitTimeout		= 10 * time.Second
	gpsOcrConfidence	= 0.6
)

/**
 * 修改微信小程序的GPS位置
 * 1. 在小程序界面上找到GPS定位元素并点击
 * 2. 等待调整到地址选择界面
 * 3. 通过OCR读取地址选择界面上的文本，找到与Keyword匹配的地址选项
 * 4. 点击匹配的地址选项，完成GPS位置修改
 * 5. 如果过程中出现任何异常（例如未找到GPS元素、未找到匹配的地址选项等），返回错误
 */
func ChangeGPSLocation(rt script.ModuleRuntime, location PreferedLocation) error {
	logger := rt.Logger()
	logger.Debug("ChangeGPSLocation start: location=%+v", location)

	win, exist := rt.GuiWindow(location.GuiId)
	if !exist {
		logger.Error("ChangeGPSLocation failed: gui window not found, guiId=%s", location.GuiId)
		return fmt.Errorf("gui windows {%s} not exist", location.GuiId)
	}
	body, err := win.BodyLocator()
	if err != nil {
		logger.Error("ChangeGPSLocation failed: BodyLocator error, guiId=%s, err=%v", location.GuiId, err)
		return fmt.Errorf("failed to locate body for gui window {%s}: %v", location.GuiId, err)
	}

	header := body.GetBodyRect().HeaderPercent(15)
	headLocator := body.SubLocator(header.Position(), header.Size())
	gpsLocators, err := headLocator.VisionLocator(gpsElementDescription, nil, nil)
	if err != nil {
		logger.Error("ChangeGPSLocation failed: VisionLocator error, guiId=%s, err=%v", location.GuiId, err)
		return fmt.Errorf("failed to locate GPS element for gui window {%s}: %v", location.GuiId, err)
	}
	if len(gpsLocators) == 0 {
		logger.Error("ChangeGPSLocation failed: GPS element not found, guiId=%s", location.GuiId)
		return fmt.Errorf("GPS element not found in gui window {%s}", location.GuiId)
	}
	logger.Debug("ChangeGPSLocation GPS element found: count=%d guiId=%s", len(gpsLocators), location.GuiId)

	if err := gpsLocators[0].Click(nil); err != nil {
		logger.Error("ChangeGPSLocation failed: click GPS element error, guiId=%s, err=%v", location.GuiId, err)
		return fmt.Errorf("failed to click GPS element for gui window {%s}: %v", location.GuiId, err)
	}
	logger.Debug("ChangeGPSLocation GPS element clicked, guiId=%s", location.GuiId)

	_, err = body.WaitForVision(gpsWaitTimeout, gpsAddressDescription, nil, nil)
	if err != nil {
		logger.Error("ChangeGPSLocation failed: address selection screen did not appear, guiId=%s, err=%v", location.GuiId, err)
		return fmt.Errorf("address selection screen did not appear for gui window {%s}: %v", location.GuiId, err)
	}
	logger.Debug("ChangeGPSLocation address selection screen loaded, guiId=%s", location.GuiId)

	ocrResult, err := body.OcrRead(gpsOcrConfidence)
	if err != nil {
		logger.Error("ChangeGPSLocation failed: OCR read error, guiId=%s, err=%v", location.GuiId, err)
		return fmt.Errorf("failed to read address options for gui window {%s}: %v", location.GuiId, err)
	}

	for _, t := range ocrResult.Texts {
		if strings.Contains(t.Text, location.Keyword) {
			pnt := t.Rect.CenterPoint()
			logger.Debug("ChangeGPSLocation match found: keyword=%s text=%s point=(%d,%d)", location.Keyword, t.Text, pnt.X, pnt.Y)
			if err := body.Click(&pnt); err != nil {
				logger.Error("ChangeGPSLocation failed: click address error, guiId=%s, err=%v", location.GuiId, err)
				return fmt.Errorf("failed to click matched address for gui window {%s}: %v", location.GuiId, err)
			}
			logger.Info("ChangeGPSLocation success: guiId=%s keyword=%s matched=%s", location.GuiId, location.Keyword, t.Text)
			return nil
		}
	}

	logger.Error("ChangeGPSLocation failed: no matching address found, guiId=%s, keyword=%s", location.GuiId, location.Keyword)
	return fmt.Errorf("no address matching keyword {%s} found in gui window {%s}", location.Keyword, location.GuiId)
}

// File: wxapputils\check_wxapp_ready.go
type PopupDialogInfo struct {
	Border		basic.Rect	`json:"border"`
	CloseBtn	basic.Rect	`json:"close_btn"`
}

type AppReadyData struct {
	AppName	string	`json:"app_name"`
	GuId	string	`json:"guid"`
}

const (
	popupDetectInstruction	= "Detect all modal popups that block interaction in the WeChat mini program window. Return each popup border and its close button area. Return an empty array if no popup exists."
	popupDetectSchema	= `[
  {
    "border": {"x": 0, "y": 0, "width": 0, "height": 0},
    "close_btn": {"x": 0, "y": 0, "width": 0, "height": 0}
  }
]`
	maxPopupCheckRounds	= 3
	popupClickDelay		= 150 * time.Millisecond
	popupRoundDelay		= 300 * time.Millisecond
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

	if guiId == "" {
		logger.Error("CheckWxappReady failed: guiId not set")
		return false, fmt.Errorf("guiId not set")
	}

	win, exist := rt.GuiWindow(guiId)
	if !exist {
		logger.Error("CheckWxappReady failed: gui window not found, guiId=%s", guiId)
		return false, fmt.Errorf("gui windows {%s} not exist", guiId)
	}
	body, err := win.BodyLocator()
	if err != nil {
		logger.Error("CheckWxappReady failed: BodyLocator error, guiId=%s, err=%v", guiId, err)
		return false, fmt.Errorf("failed to locate body for gui window {%s}: %v", guiId, err)
	}

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

	if err := rt.Publish("app_ready", AppReadyData{
		AppName:	win.GetWindowTitle(),
		GuId:		guiId,
	}); err != nil {
		logger.Error("CheckWxappReady failed: publish app_ready error, guiId=%s, err=%v", guiId, err)
		return false, fmt.Errorf("failed to publish app_ready for gui window {%s}: %v", guiId, err)
	}

	logger.Info("CheckWxappReady success: app is ready, guiId=%s title=%s", guiId, win.GetWindowTitle())

	return true, nil
}

