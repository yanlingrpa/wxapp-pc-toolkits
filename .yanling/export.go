package main

// AppReadyData (publish)
type Github_com__yanlingrpa__wxapp_pc_toolkits__AppReadyData struct {
	AppName	string	`json:"app_name"`
	GuId	string	`json:"guid"`
}

// PreferedLocation (method)
type Github_com__yanlingrpa__wxapp_pc_toolkits__PreferedLocation struct {
	GuiId	string	`json:"gui_id"`
	Keyword	string	`json:"keyword"`
}

// WxappPageInfo (method)
type Github_com__yanlingrpa__wxapp_pc_toolkits__WxappPageInfo struct {
	PageType	string	`json:"page_type"`	// 页面类型，例如：主页、商品页、订单页、搜索页、购物车页、用户中心页等
	Searchable	bool	`json:"searchable"`	// 页面上是否有搜索框
	Backable	bool	`json:"backable"`	// 页面上是否有返回按钮
	HeadNavCount	int	`json:"head_nav_count"`	// 顶部导航的总数量，如果没有顶部导航栏则为0
	HeadNavIndex	int	`json:"head_nav_index"`	// 如果是顶部导航页面，返回对应的导航索引；否则为-1
	FootNavCount	int	`json:"foot_nav_count"`	// 底部导航的总数量，如果没有底部导航栏则为0
	FootNavIndex	int	`json:"foot_nav_index"`	// 如果是底部导航页面，返回对应的导航索引；否则为-1
}

