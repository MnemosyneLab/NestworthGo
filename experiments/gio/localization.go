package main

type appLanguage string

const (
	english           appLanguage = "en"
	simplifiedChinese appLanguage = "zh-CN"
)

type messageID uint8

const (
	noMessage messageID = iota
	languageName
	navOverview
	navAccounts
	navAvailableFunds
	navPortfolio
	navTasks
	navHistory
	navHistoricalOverview
	navReturns
	navAssetChanges
	navDirectory
	navDataMaintenance
	navSettings
	navComponentPreview
	groupActivity
	groupInsights
	groupManage
	groupDevelopment
	sampleData
	netWorth
	assets
	liabilities
	inputMethodTest
	inputMethodHint
	previewFormHelp
	previewInstitutionName
	previewInstitutionHint
	previewNotes
	previewNotesHint
	previewNotesHelp
	fieldByteCount
	fieldByteExceeded
	fieldClearSearch
	fieldDismiss
	previewSearch
	previewSearchHint
	previewReadOnly
	previewDisabled
	previewSubmit
	previewNameRequired
	previewResults
	previewSubmissions
	previewSavedName
	previewSavedNotes
	previewQuery
	previewHarbor
	previewEvergreen
	previewWestlake
	previewNoMatches
	previewInfo
	previewInfoBody
	lightAppearance
	darkAppearance
	messageCount
)

type catalog [messageCount]string

var catalogs = map[appLanguage]*catalog{
	english: {
		languageName:           "English",
		navOverview:            "Overview",
		navAccounts:            "Accounts",
		navAvailableFunds:      "Available funds",
		navPortfolio:           "Investment assets",
		navTasks:               "Tasks",
		navHistory:             "Activity history",
		navHistoricalOverview:  "Historical overview",
		navReturns:             "Return analysis",
		navAssetChanges:        "Asset changes",
		navDirectory:           "Directory",
		navDataMaintenance:     "Data maintenance",
		navSettings:            "Settings",
		navComponentPreview:    "Component Preview",
		groupActivity:          "Activity",
		groupInsights:          "Insights",
		groupManage:            "Manage",
		groupDevelopment:       "Development",
		sampleData:             "Sample data",
		netWorth:               "Net worth",
		assets:                 "Assets",
		liabilities:            "Liabilities",
		inputMethodTest:        "Input method test",
		inputMethodHint:        "Try Chinese, English, or punctuation. Press Return for a new line.",
		previewFormHelp:        "This sample form keeps its draft for the current session.",
		previewInstitutionName: "Institution name",
		previewInstitutionHint: "Example Bank",
		previewNotes:           "Notes",
		previewNotesHint:       "Add a note.",
		previewNotesHelp:       "Press Return for a new line.",
		fieldByteCount:         "%d / %d bytes",
		fieldByteExceeded:      "%d / %d bytes. Shorten the text before submitting.",
		fieldClearSearch:       "Clear search",
		fieldDismiss:           "Close",
		previewSearch:          "Search institutions",
		previewSearchHint:      "Search sample institution names",
		previewReadOnly:        "Read-only example",
		previewDisabled:        "Disabled example",
		previewSubmit:          "Submit sample",
		previewNameRequired:    "Enter an institution name.",
		previewResults:         "Sample results",
		previewSubmissions:     "Submissions: %d",
		previewSavedName:       "Submitted name: %s",
		previewSavedNotes:      "Submitted notes: %s",
		previewQuery:           "Search query: %s",
		previewHarbor:          "Harbor Bank",
		previewEvergreen:       "Evergreen Credit Union",
		previewWestlake:        "Westlake Savings",
		previewNoMatches:       "No matching institutions",
		previewInfo:            "About this form",
		previewInfoBody:        "The form and search use sample data. Press Escape or click outside to close this panel.",
		lightAppearance:        "Light",
		darkAppearance:         "Dark",
	},
	simplifiedChinese: {
		languageName:           "简体中文",
		navOverview:            "概览",
		navAccounts:            "账户",
		navAvailableFunds:      "可用资金",
		navPortfolio:           "投资资产",
		navTasks:               "待办核查",
		navHistory:             "账务记录",
		navHistoricalOverview:  "资产回溯",
		navReturns:             "收益分析",
		navAssetChanges:        "资产变动",
		navDirectory:           "名录",
		navDataMaintenance:     "数据维护",
		navSettings:            "设置",
		navComponentPreview:    "组件预览",
		groupActivity:          "活动",
		groupInsights:          "洞察",
		groupManage:            "管理",
		groupDevelopment:       "开发",
		sampleData:             "示例数据",
		netWorth:               "净资产",
		assets:                 "资产",
		liabilities:            "负债",
		inputMethodTest:        "输入法测试",
		inputMethodHint:        "试试输入中文、英文或标点，按回车可换行。",
		previewFormHelp:        "示例表单的草稿仅在当前会话保留。",
		previewInstitutionName: "机构名称",
		previewInstitutionHint: "示例银行",
		previewNotes:           "备注",
		previewNotesHint:       "输入备注。",
		previewNotesHelp:       "按回车可换行。",
		fieldByteCount:         "%d / %d 字节",
		fieldByteExceeded:      "%d / %d 字节。请缩短文字后再提交。",
		fieldClearSearch:       "清空搜索",
		fieldDismiss:           "关闭",
		previewSearch:          "搜索机构",
		previewSearchHint:      "搜索示例机构名称",
		previewReadOnly:        "只读示例",
		previewDisabled:        "禁用示例",
		previewSubmit:          "提交示例",
		previewNameRequired:    "请输入机构名称。",
		previewResults:         "示例结果",
		previewSubmissions:     "提交次数：%d",
		previewSavedName:       "已提交名称：%s",
		previewSavedNotes:      "已提交备注：%s",
		previewQuery:           "搜索内容：%s",
		previewHarbor:          "海港银行",
		previewEvergreen:       "常青信用社",
		previewWestlake:        "西湖储蓄银行",
		previewNoMatches:       "没有匹配的机构",
		previewInfo:            "表单说明",
		previewInfoBody:        "表单和搜索使用示例数据。按 Esc 或点击面板外部即可关闭。",
		lightAppearance:        "浅色",
		darkAppearance:         "深色",
	},
}

func (language appLanguage) text(id messageID) string {
	if translated := catalogs[language]; translated != nil && translated[id] != "" {
		return translated[id]
	}
	return catalogs[english][id]
}

func (language appLanguage) alternate() appLanguage {
	if language == english {
		return simplifiedChinese
	}
	return english
}
