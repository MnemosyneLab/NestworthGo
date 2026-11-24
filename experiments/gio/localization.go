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
		previewNotesHint:       "Add a note. Press Return for a new line.",
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
		previewNotesHint:       "输入备注，按回车可换行。",
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
