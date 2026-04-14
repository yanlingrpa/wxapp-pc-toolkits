package wxapputils

import (
	"encoding/json"
	"fmt"
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

func CheckWxappReady(rt script.ModuleRuntime) (bool, error) {
	ret, exist := rt.GetVariable("wxappId")
	if !exist {
		return false, fmt.Errorf("failed to get wxappId")
	}
	guiId := ret.(string)
	win, exist := rt.GuiWindow(guiId)
	if !exist {
		return false, fmt.Errorf("gui windows {%s} not exist", guiId)
	}
	body, err := win.BodyLocator()
	if err != nil {
		return false, fmt.Errorf("failed to locate body for gui window {%s}: %v", guiId, err)
	}
	bts, err := body.Snapshot(true)
	if err != nil {
		return false, fmt.Errorf("failed to snapshot body for gui window {%s}: %v", guiId, err)
	}
	jsonRet, err := rt.Vision().Detect(bts, "promot", "请判定")
	if err != nil {
		return false, fmt.Errorf("failed to detect vision for gui window {%s}: %v", guiId, err)
	}
	var popups []PopupDialogInfo
	err = json.Unmarshal([]byte(jsonRet), &popups)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal vision result for gui window {%s}: %v", guiId, err)
	}
	// 如果检测到有弹窗，则执行关闭操作
	if len(popups) > 0 {
		for _, popup := range popups {
			// 点击关闭按钮
			x, y := popup.CloseBtn.Center()
			body.Click(&basic.Point{X: x, Y: y})
			time.Sleep(100 * time.Millisecond)
		}
	}

	rt.Publish("app_ready", AppReadyData{
		AppName: win.GetWindowTitle(),
		GuId:    guiId,
	})

	return true, nil
}
