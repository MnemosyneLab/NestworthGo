package main

type appLanguage string

const (
	english            appLanguage = "en"
	simplifiedChinese  appLanguage = "zh-CN"
	traditionalChinese appLanguage = "zh-TW"
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
	pickerEmpty
	pickerEmptyOption
	pickerClear
	pickerFilter
	pickerFilterHint
	pickerResultsKeys
	pickerInvalidDate
	pickerInvalidTime
	datePreviousMonth
	dateNextMonth
	datePreviousYear
	dateNextYear
	dateToday
	weekdaySunday
	weekdayMonday
	weekdayTuesday
	weekdayWednesday
	weekdayThursday
	weekdayFriday
	weekdaySaturday
	timeHours
	timeMinutes
	timeAM
	timePM
	timeHourDisplay
	timeTwelveDisplay
	previewCash
	previewCredit
	previewBrokerage
	previewHarborBranch
	previewAccountType
	previewInstitutionChoice
	previewEffectiveDate
	previewEffectiveTime
	previewAccountResult
	previewInstitutionResult
	previewDateResult
	previewTimeResult
	previewMondayFirst
	previewSundayFirst
	preview24Hour
	preview12Hour
	navInstitutions
	institutionsSynthetic
	institutionsSearchHint
	institutionsRefresh
	institutionsRetry
	institutionsLoading
	institutionsEmpty
	institutionsLoadError
	institutionsReloadAfterSaveError
	institutionsSaved
	institutionsEdit
	institutionsEditTitle
	institutionsEditName
	institutionsSave
	institutionsSaving
	institutionsCancel
	institutionsSaveError
	institutionsNameLength
	institutionBank
	institutionBrokerage
	institutionInsurer
	institutionExchange
	institutionEmployer
	institutionGovernment
	institutionOther
	lightAppearance
	darkAppearance
	colorThemeLabel
	indigoAccent
	lavenderAccent
	mintAccent
	skyAccent
	peachAccent
	messageCount
)

type catalog [messageCount]string

var catalogs = map[appLanguage]*catalog{
	english: {
		languageName:             "English",
		navOverview:              "Overview",
		navAccounts:              "Accounts",
		navAvailableFunds:        "Available funds",
		navPortfolio:             "Investment assets",
		navTasks:                 "Tasks",
		navHistory:               "Activity history",
		navHistoricalOverview:    "Historical overview",
		navReturns:               "Return analysis",
		navAssetChanges:          "Asset changes",
		navDirectory:             "Directory",
		navDataMaintenance:       "Data maintenance",
		navSettings:              "Settings",
		navComponentPreview:      "Component Preview",
		groupActivity:            "Activity",
		groupInsights:            "Insights",
		groupManage:              "Manage",
		groupDevelopment:         "Development",
		sampleData:               "Sample data",
		netWorth:                 "Net worth",
		assets:                   "Assets",
		liabilities:              "Liabilities",
		inputMethodTest:          "Input method test",
		inputMethodHint:          "Try Chinese, English, or punctuation. Press Return for a new line.",
		previewFormHelp:          "This sample form keeps its draft for the current session.",
		previewInstitutionName:   "Institution name",
		previewInstitutionHint:   "Example Bank",
		previewNotes:             "Notes",
		previewNotesHint:         "Add a note.",
		previewNotesHelp:         "Press Return for a new line.",
		fieldByteCount:           "%d / %d bytes",
		fieldByteExceeded:        "%d / %d bytes. Shorten the text before submitting.",
		fieldClearSearch:         "Clear search",
		fieldDismiss:             "Close",
		previewSearch:            "Search institutions",
		previewSearchHint:        "Search sample institution names",
		previewReadOnly:          "Read-only example",
		previewDisabled:          "Disabled example",
		previewSubmit:            "Submit sample",
		previewNameRequired:      "Enter an institution name.",
		previewResults:           "Sample results",
		previewSubmissions:       "Submissions: %d",
		previewSavedName:         "Submitted name: %s",
		previewSavedNotes:        "Submitted notes: %s",
		previewQuery:             "Search query: %s",
		previewHarbor:            "Harbor Bank",
		previewEvergreen:         "Evergreen Credit Union",
		previewWestlake:          "Westlake Savings",
		previewNoMatches:         "No matching institutions",
		previewInfo:              "About this form",
		previewInfoBody:          "The form and search use sample data. Press Escape or click outside to close this panel.",
		pickerEmpty:              "No selection",
		pickerEmptyOption:        "None",
		pickerClear:              "Clear selection",
		pickerFilter:             "Filter options",
		pickerFilterHint:         "Type to filter options",
		pickerResultsKeys:        "Alt+↓ focuses results. Alt+↑ returns to search.",
		pickerInvalidDate:        "Invalid date",
		pickerInvalidTime:        "Invalid time",
		datePreviousMonth:        "Previous month",
		dateNextMonth:            "Next month",
		datePreviousYear:         "Previous year",
		dateNextYear:             "Next year",
		dateToday:                "Today, %s",
		weekdaySunday:            "Sun",
		weekdayMonday:            "Mon",
		weekdayTuesday:           "Tue",
		weekdayWednesday:         "Wed",
		weekdayThursday:          "Thu",
		weekdayFriday:            "Fri",
		weekdaySaturday:          "Sat",
		timeHours:                "Hours",
		timeMinutes:              "Minutes",
		timeAM:                   "AM",
		timePM:                   "PM",
		timeHourDisplay:          "%d %s",
		timeTwelveDisplay:        "%d:%02d %s",
		previewCash:              "Cash account",
		previewCredit:            "Credit card",
		previewBrokerage:         "Brokerage account",
		previewHarborBranch:      "Harbor Branch %02d",
		previewAccountType:       "Account type",
		previewInstitutionChoice: "Choose institution",
		previewEffectiveDate:     "Effective date",
		previewEffectiveTime:     "Effective time",
		previewAccountResult:     "Account type: %s",
		previewInstitutionResult: "Institution: %s",
		previewDateResult:        "Effective date: %s",
		previewTimeResult:        "Effective time: %s",
		previewMondayFirst:       "Monday first",
		previewSundayFirst:       "Sunday first",
		preview24Hour:            "24-hour time",
		preview12Hour:            "12-hour time",
		lightAppearance:          "Light",
		darkAppearance:           "Dark",
		colorThemeLabel:          "Color theme: %s",
		indigoAccent:             "Indigo",
		lavenderAccent:           "Lavender",
		mintAccent:               "Mint",
		skyAccent:                "Sky",
		peachAccent:              "Peach",

		navInstitutions:                  "Institutions",
		institutionsSynthetic:            "Synthetic test household",
		institutionsSearchHint:           "Filter by institution name",
		institutionsRefresh:              "Reload",
		institutionsRetry:                "Retry reload",
		institutionsLoading:              "Loading institutions…",
		institutionsEmpty:                "This household has no active institutions.",
		institutionsLoadError:            "Could not load institutions.",
		institutionsReloadAfterSaveError: "The name was saved. Retry to reload the list.",
		institutionsSaved:                "Institution saved.",
		institutionsEdit:                 "Edit",
		institutionsEditTitle:            "Edit institution",
		institutionsEditName:             "Edit %s",
		institutionsSave:                 "Save",
		institutionsSaving:               "Saving and reloading…",
		institutionsCancel:               "Cancel",
		institutionsSaveError:            "Could not save the institution. Try again.",
		institutionsNameLength:           "Use %d characters or fewer.",
		institutionBank:                  "Bank",
		institutionBrokerage:             "Brokerage",
		institutionInsurer:               "Insurer",
		institutionExchange:              "Exchange",
		institutionEmployer:              "Employer",
		institutionGovernment:            "Government",
		institutionOther:                 "Other",
	},
	simplifiedChinese: {
		languageName:             "简体中文",
		navOverview:              "概览",
		navAccounts:              "账户",
		navAvailableFunds:        "可用资金",
		navPortfolio:             "投资资产",
		navTasks:                 "待办核查",
		navHistory:               "账务记录",
		navHistoricalOverview:    "资产回溯",
		navReturns:               "收益分析",
		navAssetChanges:          "资产变动",
		navDirectory:             "名录",
		navDataMaintenance:       "数据维护",
		navSettings:              "设置",
		navComponentPreview:      "组件预览",
		groupActivity:            "活动",
		groupInsights:            "洞察",
		groupManage:              "管理",
		groupDevelopment:         "开发",
		sampleData:               "示例数据",
		netWorth:                 "净资产",
		assets:                   "资产",
		liabilities:              "负债",
		inputMethodTest:          "输入法测试",
		inputMethodHint:          "试试输入中文、英文或标点，按回车可换行。",
		previewFormHelp:          "示例表单的草稿仅在当前会话保留。",
		previewInstitutionName:   "机构名称",
		previewInstitutionHint:   "示例银行",
		previewNotes:             "备注",
		previewNotesHint:         "输入备注。",
		previewNotesHelp:         "按回车可换行。",
		fieldByteCount:           "%d / %d 字节",
		fieldByteExceeded:        "%d / %d 字节。请缩短文字后再提交。",
		fieldClearSearch:         "清空搜索",
		fieldDismiss:             "关闭",
		previewSearch:            "搜索机构",
		previewSearchHint:        "搜索示例机构名称",
		previewReadOnly:          "只读示例",
		previewDisabled:          "禁用示例",
		previewSubmit:            "提交示例",
		previewNameRequired:      "请输入机构名称。",
		previewResults:           "示例结果",
		previewSubmissions:       "提交次数：%d",
		previewSavedName:         "已提交名称：%s",
		previewSavedNotes:        "已提交备注：%s",
		previewQuery:             "搜索内容：%s",
		previewHarbor:            "海港银行",
		previewEvergreen:         "常青信用社",
		previewWestlake:          "西湖储蓄银行",
		previewNoMatches:         "没有匹配的机构",
		previewInfo:              "表单说明",
		previewInfoBody:          "表单和搜索使用示例数据。按 Esc 或点击面板外部即可关闭。",
		pickerEmpty:              "未选择",
		pickerEmptyOption:        "无",
		pickerClear:              "清除选择",
		pickerFilter:             "筛选选项",
		pickerFilterHint:         "输入文字筛选",
		pickerResultsKeys:        "Alt+↓ 移到结果列表，Alt+↑ 返回搜索框。",
		pickerInvalidDate:        "日期无效",
		pickerInvalidTime:        "时间无效",
		datePreviousMonth:        "上个月",
		dateNextMonth:            "下个月",
		datePreviousYear:         "上一年",
		dateNextYear:             "下一年",
		dateToday:                "今天，%s",
		weekdaySunday:            "日",
		weekdayMonday:            "一",
		weekdayTuesday:           "二",
		weekdayWednesday:         "三",
		weekdayThursday:          "四",
		weekdayFriday:            "五",
		weekdaySaturday:          "六",
		timeHours:                "小时",
		timeMinutes:              "分钟",
		timeAM:                   "上午",
		timePM:                   "下午",
		timeHourDisplay:          "%[2]s%[1]d",
		timeTwelveDisplay:        "%[3]s%[1]d:%02[2]d",
		previewCash:              "现金账户",
		previewCredit:            "信用卡",
		previewBrokerage:         "证券账户",
		previewHarborBranch:      "海港分行 %02d",
		previewAccountType:       "账户类型",
		previewInstitutionChoice: "选择机构",
		previewEffectiveDate:     "生效日期",
		previewEffectiveTime:     "生效时间",
		previewAccountResult:     "账户类型：%s",
		previewInstitutionResult: "机构：%s",
		previewDateResult:        "生效日期：%s",
		previewTimeResult:        "生效时间：%s",
		previewMondayFirst:       "周一开始",
		previewSundayFirst:       "周日开始",
		preview24Hour:            "24 小时制",
		preview12Hour:            "12 小时制",
		lightAppearance:          "浅色",
		darkAppearance:           "深色",
		colorThemeLabel:          "颜色主题：%s",
		indigoAccent:             "靛蓝",
		lavenderAccent:           "薰衣草",
		mintAccent:               "薄荷",
		skyAccent:                "天空",
		peachAccent:              "蜜桃",

		navInstitutions:                  "机构",
		institutionsSynthetic:            "合成测试家庭",
		institutionsSearchHint:           "按机构名称筛选",
		institutionsRefresh:              "重新加载",
		institutionsRetry:                "重试加载",
		institutionsLoading:              "正在加载机构…",
		institutionsEmpty:                "此家庭没有启用的机构。",
		institutionsLoadError:            "无法加载机构。",
		institutionsReloadAfterSaveError: "名称已保存。请重试加载列表。",
		institutionsSaved:                "机构已保存。",
		institutionsEdit:                 "编辑",
		institutionsEditTitle:            "编辑机构",
		institutionsEditName:             "编辑 %s",
		institutionsSave:                 "保存",
		institutionsSaving:               "正在保存并重新加载…",
		institutionsCancel:               "取消",
		institutionsSaveError:            "无法保存机构。请重试。",
		institutionsNameLength:           "名称不得超过 %d 个字符。",
		institutionBank:                  "银行",
		institutionBrokerage:             "券商",
		institutionInsurer:               "保险机构",
		institutionExchange:              "交易所",
		institutionEmployer:              "雇主",
		institutionGovernment:            "政府机构",
		institutionOther:                 "其他",
	},
	traditionalChinese: {
		languageName:             "繁體中文",
		navOverview:              "總覽",
		navAccounts:              "帳戶",
		navAvailableFunds:        "可用資金",
		navPortfolio:             "投資資產",
		navTasks:                 "待辦核查",
		navHistory:               "帳務紀錄",
		navHistoricalOverview:    "資產回溯",
		navReturns:               "收益分析",
		navAssetChanges:          "資產變動",
		navDirectory:             "名錄",
		navDataMaintenance:       "資料維護",
		navSettings:              "設定",
		navComponentPreview:      "元件預覽",
		groupActivity:            "活動",
		groupInsights:            "洞察",
		groupManage:              "管理",
		groupDevelopment:         "開發",
		sampleData:               "示範資料",
		netWorth:                 "淨資產",
		assets:                   "資產",
		liabilities:              "負債",
		inputMethodTest:          "輸入法測試",
		inputMethodHint:          "試著輸入中文、英文或標點符號。按 Enter 即可換行。",
		previewFormHelp:          "示範表單的草稿會保留到關閉應用程式為止。",
		previewInstitutionName:   "機構名稱",
		previewInstitutionHint:   "示範銀行",
		previewNotes:             "備註",
		previewNotesHint:         "輸入備註。",
		previewNotesHelp:         "按 Enter 即可換行。",
		fieldByteCount:           "%d / %d 位元組",
		fieldByteExceeded:        "%d / %d 位元組。請縮短文字後再送出。",
		fieldClearSearch:         "清除搜尋",
		fieldDismiss:             "關閉",
		previewSearch:            "搜尋機構",
		previewSearchHint:        "搜尋示範機構名稱",
		previewReadOnly:          "唯讀範例",
		previewDisabled:          "停用範例",
		previewSubmit:            "送出範例",
		previewNameRequired:      "請輸入機構名稱。",
		previewResults:           "示範結果",
		previewSubmissions:       "送出次數：%d",
		previewSavedName:         "已送出的名稱：%s",
		previewSavedNotes:        "已送出的備註：%s",
		previewQuery:             "搜尋文字：%s",
		previewHarbor:            "海港銀行",
		previewEvergreen:         "常青信用合作社",
		previewWestlake:          "西湖儲蓄銀行",
		previewNoMatches:         "找不到符合條件的機構",
		previewInfo:              "表單說明",
		previewInfoBody:          "表單與搜尋均使用示範資料。按 Esc 或點選面板外即可關閉。",
		pickerEmpty:              "尚未選擇",
		pickerEmptyOption:        "無",
		pickerClear:              "清除選擇",
		pickerFilter:             "篩選選項",
		pickerFilterHint:         "輸入文字以篩選",
		pickerResultsKeys:        "Alt+↓ 移至結果清單，Alt+↑ 返回搜尋欄。",
		pickerInvalidDate:        "日期無效",
		pickerInvalidTime:        "時間無效",
		datePreviousMonth:        "上個月",
		dateNextMonth:            "下個月",
		datePreviousYear:         "上一年",
		dateNextYear:             "下一年",
		dateToday:                "今天，%s",
		weekdaySunday:            "日",
		weekdayMonday:            "一",
		weekdayTuesday:           "二",
		weekdayWednesday:         "三",
		weekdayThursday:          "四",
		weekdayFriday:            "五",
		weekdaySaturday:          "六",
		timeHours:                "小時",
		timeMinutes:              "分鐘",
		timeAM:                   "上午",
		timePM:                   "下午",
		timeHourDisplay:          "%[2]s%[1]d",
		timeTwelveDisplay:        "%[3]s%[1]d:%02[2]d",
		previewCash:              "現金帳戶",
		previewCredit:            "信用卡",
		previewBrokerage:         "券商帳戶",
		previewHarborBranch:      "海港分行 %02d",
		previewAccountType:       "帳戶類型",
		previewInstitutionChoice: "選擇機構",
		previewEffectiveDate:     "生效日期",
		previewEffectiveTime:     "生效時間",
		previewAccountResult:     "帳戶類型：%s",
		previewInstitutionResult: "機構：%s",
		previewDateResult:        "生效日期：%s",
		previewTimeResult:        "生效時間：%s",
		previewMondayFirst:       "星期一開始",
		previewSundayFirst:       "星期日開始",
		preview24Hour:            "24 小時制",
		preview12Hour:            "12 小時制",
		lightAppearance:          "淺色",
		darkAppearance:           "深色",
		colorThemeLabel:          "顏色主題：%s",
		indigoAccent:             "靛藍",
		lavenderAccent:           "薰衣草",
		mintAccent:               "薄荷",
		skyAccent:                "天空",
		peachAccent:              "蜜桃",

		navInstitutions:                  "機構",
		institutionsSynthetic:            "測試用家庭帳本",
		institutionsSearchHint:           "依機構名稱篩選",
		institutionsRefresh:              "重新載入",
		institutionsRetry:                "重試載入",
		institutionsLoading:              "正在載入機構…",
		institutionsEmpty:                "這個家庭目前沒有啟用的機構。",
		institutionsLoadError:            "無法載入機構。",
		institutionsReloadAfterSaveError: "名稱已儲存。請重試載入清單。",
		institutionsSaved:                "機構已儲存。",
		institutionsEdit:                 "編輯",
		institutionsEditTitle:            "編輯機構",
		institutionsEditName:             "編輯 %s",
		institutionsSave:                 "儲存",
		institutionsSaving:               "正在儲存並重新載入…",
		institutionsCancel:               "取消",
		institutionsSaveError:            "無法儲存機構。請再試一次。",
		institutionsNameLength:           "名稱不得超過 %d 個字元。",
		institutionBank:                  "銀行",
		institutionBrokerage:             "券商",
		institutionInsurer:               "保險機構",
		institutionExchange:              "交易所",
		institutionEmployer:              "僱主",
		institutionGovernment:            "政府機構",
		institutionOther:                 "其他",
	},
}

func (language appLanguage) text(id messageID) string {
	if translated := catalogs[language]; translated != nil && translated[id] != "" {
		return translated[id]
	}
	return catalogs[english][id]
}

func (language appLanguage) next() appLanguage {
	switch language {
	case english:
		return traditionalChinese
	case traditionalChinese:
		return simplifiedChinese
	default:
		return english
	}
}

func (language appLanguage) usesChineseDates() bool {
	return language == simplifiedChinese || language == traditionalChinese
}
