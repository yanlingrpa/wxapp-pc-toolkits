package wxapputils

import (
	"fmt"
	"strings"
	"time"

	"yanlingrpa.com/yanling/protocol/script"
)

type PreferedLocation struct {
	GuiId   string `json:"gui_id"`
	Keyword string `json:"keyword"`
}

const (
	gpsElementDescription = "GPS location or position button, address entry field, or location icon in the WeChat mini program"
	gpsAddressDescription = "address selection list or location picker showing candidate addresses"
	gpsWaitTimeout        = 10 * time.Second
	gpsOcrConfidence      = 0.6
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

	// Step 1: Find GPS positioning element and click it.
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

	// Step 2: Wait for the address selection screen to load.
	_, err = body.WaitForVision(gpsWaitTimeout, gpsAddressDescription, nil, nil)
	if err != nil {
		logger.Error("ChangeGPSLocation failed: address selection screen did not appear, guiId=%s, err=%v", location.GuiId, err)
		return fmt.Errorf("address selection screen did not appear for gui window {%s}: %v", location.GuiId, err)
	}
	logger.Debug("ChangeGPSLocation address selection screen loaded, guiId=%s", location.GuiId)

	// Step 3: OCR the address selection screen to find a match for Keyword.
	ocrResult, err := body.OcrRead(gpsOcrConfidence)
	if err != nil {
		logger.Error("ChangeGPSLocation failed: OCR read error, guiId=%s, err=%v", location.GuiId, err)
		return fmt.Errorf("failed to read address options for gui window {%s}: %v", location.GuiId, err)
	}

	// Step 4: Click the first address option that contains the keyword.
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
