package wxapputils

import (
	"encoding/json"
	"fmt"
	"strings"

	"yanlingrpa.com/yanling/protocol/script"
)

const (
	pageInfoDetectInstruction = "Analyze the screenshot of a WeChat mini program page and infer page semantics and navigation state. Return only a JSON object with fields: page_type, searchable, backable, head_nav_count, head_nav_index, foot_nav_count, foot_nav_index. page_type should be a concise Chinese page type such as 主页、商品页、订单页、搜索页、购物车页、用户中心页; if uncertain use 未知页面. Use -1 for nav index when not on that nav system."
	pageInfoDetectSchema      = `{
  "type": "object",
  "properties": {
    "page_type": {"type": "string"},
    "searchable": {"type": "boolean"},
    "backable": {"type": "boolean"},
    "head_nav_count": {"type": "integer", "minimum": 0},
    "head_nav_index": {"type": "integer"},
    "foot_nav_count": {"type": "integer", "minimum": 0},
    "foot_nav_index": {"type": "integer"}
  },
  "required": [
    "page_type",
    "searchable",
    "backable",
    "head_nav_count",
    "head_nav_index",
    "foot_nav_count",
    "foot_nav_index"
  ],
  "additionalProperties": false
}`
)

type pageInfoEnvelope struct {
	PageInfo WxappPageInfo `json:"page_info"`
}

func stripJSONCodeFence(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return trimmed
	}

	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}

	parts := strings.Split(trimmed, "\n")
	if len(parts) < 2 {
		return trimmed
	}

	if strings.HasPrefix(parts[0], "```") {
		parts = parts[1:]
	}
	if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "```" {
		parts = parts[:len(parts)-1]
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func parsePageInfoDetectResult(jsonRet string) (WxappPageInfo, error) {
	trimmed := stripJSONCodeFence(jsonRet)
	if trimmed == "" || trimmed == "null" {
		return WxappPageInfo{}, fmt.Errorf("empty detect result")
	}

	var pageInfo WxappPageInfo
	if err := json.Unmarshal([]byte(trimmed), &pageInfo); err == nil {
		return pageInfo, nil
	}

	var envelope pageInfoEnvelope
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil {
		return envelope.PageInfo, nil
	}

	return WxappPageInfo{}, fmt.Errorf("unexpected detect result: %s", trimmed)
}

func normalizePageInfo(info WxappPageInfo) WxappPageInfo {
	if strings.TrimSpace(info.PageType) == "" {
		info.PageType = "未知页面"
	}

	if info.HeadNavCount <= 0 {
		info.HeadNavCount = 0
		info.HeadNavIndex = -1
	} else if info.HeadNavIndex < 0 || info.HeadNavIndex >= info.HeadNavCount {
		info.HeadNavIndex = -1
	}

	if info.FootNavCount <= 0 {
		info.FootNavCount = 0
		info.FootNavIndex = -1
	} else if info.FootNavIndex < 0 || info.FootNavIndex >= info.FootNavCount {
		info.FootNavIndex = -1
	}

	return info
}

// WxappPageInfo represents the information of the current page in the WeChat mini program.
type WxappPageInfo struct {
	PageType     string `json:"page_type"`      // 页面类型，例如：主页、商品页、订单页、搜索页、购物车页、用户中心页等
	Searchable   bool   `json:"searchable"`     // 页面上是否有搜索框
	Backable     bool   `json:"backable"`       // 页面上是否有返回按钮
	HeadNavCount int    `json:"head_nav_count"` // 顶部导航的总数量，如果没有顶部导航栏则为0
	HeadNavIndex int    `json:"head_nav_index"` // 如果是顶部导航页面，返回对应的导航索引；否则为-1
	FootNavCount int    `json:"foot_nav_count"` // 底部导航的总数量，如果没有底部导航栏则为0
	FootNavIndex int    `json:"foot_nav_index"` // 如果是底部导航页面，返回对应的导航索引；否则为-1
}

// 获取当前页面信息
func GetPageInfo(rt script.ModuleRuntime, guiId string) (WxappPageInfo, error) {
	logger := rt.Logger()
	logger.Debug("GetPageInfo start: guiId=%s", guiId)

	if guiId == "" {
		logger.Error("GetPageInfo failed: guiId not set")
		return WxappPageInfo{}, fmt.Errorf("guiId not set")
	}

	win, exist := rt.OsGuiWindow(guiId)
	if !exist {
		logger.Error("GetPageInfo failed: gui window not found, guiId=%s", guiId)
		return WxappPageInfo{}, fmt.Errorf("gui windows {%s} not exist", guiId)
	}

	body, err := win.BodyLocator()
	if err != nil {
		logger.Error("GetPageInfo failed: BodyLocator error, guiId=%s, err=%v", guiId, err)
		return WxappPageInfo{}, fmt.Errorf("failed to locate body for gui window {%s}: %v", guiId, err)
	}

	bts, err := body.Snapshot(true)
	if err != nil {
		logger.Error("GetPageInfo failed: snapshot error, guiId=%s, err=%v", guiId, err)
		return WxappPageInfo{}, fmt.Errorf("failed to snapshot body for gui window {%s}: %v", guiId, err)
	}
	logger.Debug("GetPageInfo snapshot captured: guiId=%s bytes=%d", guiId, len(bts))

	jsonRet, err := rt.VisionWorker().Detect(bts, pageInfoDetectInstruction, pageInfoDetectSchema)
	if err != nil {
		logger.Error("GetPageInfo failed: vision detect error, guiId=%s, err=%v", guiId, err)
		return WxappPageInfo{}, fmt.Errorf("failed to detect page info for gui window {%s}: %v", guiId, err)
	}

	pageInfo, err := parsePageInfoDetectResult(jsonRet)
	if err != nil {
		logger.Error("GetPageInfo failed: detect result parse error, guiId=%s, err=%v", guiId, err)
		return WxappPageInfo{}, fmt.Errorf("failed to parse page info detect result for gui window {%s}: %v", guiId, err)
	}

	pageInfo = normalizePageInfo(pageInfo)
	logger.Info(
		"GetPageInfo success: guiId=%s pageType=%s searchable=%v backable=%v headNav=%d/%d footNav=%d/%d",
		guiId,
		pageInfo.PageType,
		pageInfo.Searchable,
		pageInfo.Backable,
		pageInfo.HeadNavIndex,
		pageInfo.HeadNavCount,
		pageInfo.FootNavIndex,
		pageInfo.FootNavCount,
	)

	return pageInfo, nil
}
