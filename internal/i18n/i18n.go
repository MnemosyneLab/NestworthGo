// Package i18n provides the small, explicit locale catalog used by the UI
// MVP. Keeping the catalog in Go avoids adding a runtime translation service
// before the application has a durable domain layer.
package i18n

import (
	"os"
	"sort"
	"strings"

	"github.com/waltwang/nestworth-go/internal/settings"
)

type Translator struct {
	language settings.Language
}

func New(language settings.Language) *Translator {
	translator := &Translator{}
	translator.SetLanguage(language)
	return translator
}

func (t *Translator) SetLanguage(language settings.Language) {
	if language == settings.LanguageSystem {
		language = ResolveSystemLanguage()
	}
	if _, ok := catalogs[language]; !ok {
		language = settings.LanguageEnglish
	}
	t.language = language
}

func (t *Translator) Language() settings.Language {
	if t == nil || t.language == "" {
		return settings.LanguageEnglish
	}
	return t.language
}

func (t *Translator) T(key string) string {
	if t == nil {
		return catalogs[settings.LanguageEnglish][key]
	}
	if value := catalogs[t.Language()][key]; value != "" {
		return value
	}
	return catalogs[settings.LanguageEnglish][key]
}

func ResolveSystemLanguage() settings.Language {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		if value := os.Getenv(key); value != "" {
			return languageFromLocale(value)
		}
	}
	return settings.LanguageEnglish
}

func languageFromLocale(value string) settings.Language {
	value = strings.ToLower(strings.TrimSpace(strings.Split(value, ":")[0]))
	value = strings.ReplaceAll(value, "_", "-")
	if strings.HasPrefix(value, "zh-tw") || strings.HasPrefix(value, "zh-hk") || strings.HasPrefix(value, "zh-mo") {
		return settings.LanguageZhTW
	}
	if strings.HasPrefix(value, "zh-") {
		return settings.LanguageZhCN
	}
	if strings.HasPrefix(value, "zh") {
		return settings.LanguageZhCN
	}
	return settings.LanguageEnglish
}

func CatalogKeys() []string {
	seen := make(map[string]struct{})
	for _, catalog := range catalogs {
		for key := range catalog {
			seen[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func MissingKeys(language settings.Language) []string {
	missing := make([]string, 0)
	for _, key := range CatalogKeys() {
		if catalogs[language][key] == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

var catalogs = map[settings.Language]map[string]string{
	settings.LanguageEnglish: {
		"app.name":                            "Nestworth",
		"menu.file":                           "File",
		"menu.about":                          "About",
		"about.title":                         "About Nestworth",
		"about.description":                   "A local-first personal finance desktop application for building and maintaining a personal or household balance sheet.",
		"about.version":                       "Version %s · Build %s",
		"about.license":                       "MIT License",
		"common.previewData":                  "Preview data",
		"common.previewNotice":                "This is a visual preview. Your local financial data will connect here in a later release.",
		"common.comingSoon":                   "Coming soon",
		"common.openSettings":                 "Open settings",
		"common.reset":                        "Reset to defaults",
		"common.close":                        "Close",
		"common.saved":                        "Saved locally",
		"common.system":                       "System",
		"common.light":                        "Light",
		"common.dark":                         "Dark",
		"common.invalid":                      "Invalid value",
		"common.preview":                      "Preview",
		"nav.overview":                        "Overview",
		"nav.accounts":                        "Accounts",
		"nav.activity":                        "Activity",
		"nav.analytics":                       "Analytics",
		"nav.settings":                        "Settings",
		"dashboard.eyebrow":                   "HOUSEHOLD SNAPSHOT",
		"dashboard.title":                     "Your financial picture, in one place.",
		"dashboard.subtitle":                  "A calm view of the balance sheet, designed to stay understandable as your household grows.",
		"dashboard.netWorth":                  "Net worth",
		"dashboard.currentTotal":              "Current total",
		"dashboard.change":                    "This month",
		"dashboard.changeValue":               "+2.8%",
		"dashboard.assets":                    "Assets",
		"dashboard.liabilities":               "Liabilities",
		"dashboard.liquidAssets":              "Liquid assets",
		"dashboard.accountsTracked":           "accounts tracked",
		"dashboard.netWorthTrend":             "Net worth trend",
		"dashboard.previewTrend":              "A lightweight preview of the view that will connect to historical snapshots.",
		"dashboard.allocation":                "Allocation",
		"dashboard.cash":                      "Cash & equivalents",
		"dashboard.investments":               "Investments",
		"dashboard.property":                  "Property",
		"dashboard.recentActivity":            "Recent activity",
		"dashboard.activityDescription":       "The activity ledger will explain how this picture changes over time.",
		"dashboard.noActivity":                "No live activities yet",
		"dashboard.accountsToReview":          "Accounts to review",
		"dashboard.reviewDescription":         "Keep your balance sheet fresh with a quick review rhythm.",
		"dashboard.reviewItemOne":             "2 balances are ready for a refresh",
		"dashboard.reviewItemTwo":             "1 account needs an ownership check",
		"page.accountsTitle":                  "Accounts",
		"page.accountsDescription":            "Accounts will become the durable home for balances, ownership, institutions, and groups.",
		"page.activityTitle":                  "Activity",
		"page.activityDescription":            "Activities will explain deposits, transfers, valuations, income, and fees without hiding the ledger.",
		"page.analyticsTitle":                 "Analytics",
		"page.analyticsDescription":           "Analytics will separate contributions, market movement, currency movement, income, and fees.",
		"settings.title":                      "Settings",
		"settings.subtitle":                   "Shape the way Nestworth reads and presents your household picture.",
		"settings.appearance.title":           "Appearance",
		"settings.appearance.description":     "Choose the surface and accent that make the balance sheet comfortable to return to.",
		"settings.appearance.mode":            "Appearance",
		"settings.appearance.accent":          "Color theme",
		"settings.language.title":             "Language & region",
		"settings.language.description":       "Keep dates, week boundaries, and local time aligned with how you work.",
		"settings.language.language":          "Language",
		"settings.language.timezone":          "Timezone",
		"settings.language.weekStart":         "First day of week",
		"settings.language.dateFormat":        "Date format",
		"settings.language.timeFormat":        "Time format",
		"settings.numbers.title":              "Currency & numbers",
		"settings.numbers.description":        "These choices affect presentation only; stored financial precision remains exact.",
		"settings.numbers.currency":           "Primary display currency",
		"settings.numbers.decimal":            "Decimal separator",
		"settings.numbers.grouping":           "Grouping separator",
		"settings.numbers.places":             "Decimal places",
		"settings.numbers.preview":            "Formatting preview",
		"settings.reset.title":                "Reset preferences?",
		"settings.reset.body":                 "All appearance and localization choices will return to their defaults.",
		"settings.reset.confirm":              "Reset",
		"settings.saveError":                  "Your change is active for this session, but could not be saved locally.",
		"option.appearance.system":            "System",
		"option.appearance.light":             "Light",
		"option.appearance.dark":              "Dark",
		"option.accent.nestworth":             "Nestworth",
		"option.accent.ocean":                 "Ocean",
		"option.accent.forest":                "Forest",
		"option.accent.amber":                 "Amber",
		"option.accent.rose":                  "Rose",
		"option.language.system":              "System",
		"option.language.en":                  "English",
		"option.language.zhCN":                "简体中文",
		"option.language.zhTW":                "正體中文",
		"option.timezone.system":              "System timezone",
		"option.week.monday":                  "Monday",
		"option.week.sunday":                  "Sunday",
		"option.date.iso":                     "YYYY-MM-DD",
		"option.date.dayFirst":                "DD/MM/YYYY",
		"option.date.monthFirst":              "MM/DD/YYYY",
		"option.date.localized":               "Localized",
		"option.time.24h":                     "24-hour",
		"option.time.12h":                     "12-hour",
		"option.grouping.none":                "None",
		"option.grouping.space":               "Space",
		"format.sampleLabel":                  "Example",
		"format.todayLabel":                   "Current local time",
		"nav.members":                         "Members",
		"nav.institutions":                    "Institutions",
		"nav.groups":                          "Groups",
		"page.membersTitle":                   "Members",
		"page.membersDescription":             "People used for exact account ownership.",
		"page.institutionsTitle":              "Institutions",
		"page.institutionsDescription":        "Places where accounts are held.",
		"page.groupsTitle":                    "Groups",
		"page.groupsDescription":              "Flexible household organization for accounts.",
		"startup.blockedTitle":                "Local database unavailable",
		"startup.blockedDescription":          "Nestworth kept financial writes disabled because the local database could not be opened safely.",
		"startup.readOnly":                    "Business data is read-only",
		"onboarding.title":                    "Start your Household",
		"onboarding.description":              "Create one local balance sheet and add at least one member.",
		"onboarding.householdName":            "Household name",
		"onboarding.householdNamePlaceholder": "Wang Household",
		"onboarding.baseCurrency":             "Base currency",
		"onboarding.members":                  "Members",
		"onboarding.memberNamesPlaceholder":   "Alice, Bob",
		"onboarding.create":                   "Create Household",
		"overview.assets":                     "Assets",
		"overview.liabilities":                "Liabilities",
		"overview.netWorth":                   "Net worth",
		"overview.byCategory":                 "By category",
		"overview.byMember":                   "By member",
		"overview.byInstitution":              "By institution",
		"overview.byGroup":                    "By group",
		"overview.emptyTitle":                 "Your balance sheet is empty",
		"overview.emptyDescription":           "Add your first account to see assets, liabilities, and net worth.",
		"overview.loadError":                  "Overview could not be loaded",
		"accounts.filter":                     "Filter",
		"accounts.filterAll":                  "All accounts",
		"accounts.createTitle":                "Add an account",
		"accounts.createDescription":          "Record a current balance or manual value without losing exact ownership.",
		"accounts.create":                     "Add account",
		"accounts.name":                       "Name",
		"accounts.amount":                     "Initial value",
		"accounts.category":                   "Category",
		"accounts.secondaryCategory":          "Subcategory",
		"accounts.trackingMode":               "Tracking mode",
		"accounts.owner":                      "Owner",
		"accounts.emptyTitle":                 "No accounts yet",
		"accounts.emptyDescription":           "Use Add account to record an asset or liability.",
		"accounts.noValue":                    "No current value",
		"accounts.archive":                    "Archive",
		"accounts.loadError":                  "Accounts could not be loaded",
		"members.createTitle":                 "Add a member",
		"members.createDescription":           "Members are used for exact ownership and allocation.",
		"members.name":                        "Name",
		"members.loadError":                   "Members could not be loaded",
		"institutions.createTitle":            "Add an institution",
		"institutions.createDescription":      "Keep track of where accounts are held.",
		"institutions.name":                   "Name",
		"institutions.loadError":              "Institutions could not be loaded",
		"groups.createTitle":                  "Add a group",
		"groups.createDescription":            "Organize accounts by a household-defined purpose.",
		"groups.name":                         "Name",
		"groups.loadError":                    "Groups could not be loaded",
		"common.add":                          "Add",
		"common.archive":                      "Archive",
		"common.active":                       "Active",
		"common.archived":                     "Archived",
	},
	settings.LanguageZhCN: {
		"app.name":                        "Nestworth",
		"menu.file":                       "文件",
		"menu.about":                      "关于",
		"about.title":                     "关于 Nestworth",
		"about.description":               "一款以本地优先为理念、用于建立和维护个人或家庭资产负债表的财务桌面应用。",
		"about.version":                   "版本 %s · 构建 %s",
		"about.license":                   "MIT 许可证",
		"common.previewData":              "演示数据",
		"common.previewNotice":            "这是视觉预览。后续版本会在这里接入你的本地财务数据。",
		"common.comingSoon":               "即将推出",
		"common.openSettings":             "打开设置",
		"common.reset":                    "恢复默认设置",
		"common.close":                    "关闭",
		"common.saved":                    "已保存到本地",
		"common.system":                   "系统",
		"common.light":                    "浅色",
		"common.dark":                     "深色",
		"common.invalid":                  "值无效",
		"common.preview":                  "预览",
		"nav.overview":                    "概览",
		"nav.accounts":                    "账户",
		"nav.activity":                    "活动",
		"nav.analytics":                   "分析",
		"nav.settings":                    "设置",
		"dashboard.eyebrow":               "家庭资产快照",
		"dashboard.title":                 "一处看清你的财务全貌。",
		"dashboard.subtitle":              "以清晰、平静的方式查看资产负债表，并随着家庭成长逐步扩展。",
		"dashboard.netWorth":              "净资产",
		"dashboard.currentTotal":          "当前总额",
		"dashboard.change":                "本月变化",
		"dashboard.changeValue":           "+2.8%",
		"dashboard.assets":                "资产",
		"dashboard.liabilities":           "负债",
		"dashboard.liquidAssets":          "流动资产",
		"dashboard.accountsTracked":       "个账户已跟踪",
		"dashboard.netWorthTrend":         "净资产趋势",
		"dashboard.previewTrend":          "未来将连接历史快照，目前仅展示界面预览。",
		"dashboard.allocation":            "资产分配",
		"dashboard.cash":                  "现金及等价物",
		"dashboard.investments":           "投资",
		"dashboard.property":              "不动产",
		"dashboard.recentActivity":        "近期活动",
		"dashboard.activityDescription":   "活动账本会解释这张资产负债表如何随时间变化。",
		"dashboard.noActivity":            "暂时没有真实活动",
		"dashboard.accountsToReview":      "待复核账户",
		"dashboard.reviewDescription":     "通过轻量的复核节奏，让资产负债表保持新鲜。",
		"dashboard.reviewItemOne":         "2 个余额可以更新",
		"dashboard.reviewItemTwo":         "1 个账户需要检查所有权",
		"page.accountsTitle":              "账户",
		"page.accountsDescription":        "账户将成为管理余额、所有权、机构和分组的长期入口。",
		"page.activityTitle":              "活动",
		"page.activityDescription":        "活动会解释存入、转账、估值、收入和费用，同时保留清晰的账本。",
		"page.analyticsTitle":             "分析",
		"page.analyticsDescription":       "分析会区分投入、市场变化、汇率变化、收入和费用。",
		"settings.title":                  "设置",
		"settings.subtitle":               "调整 Nestworth 展示家庭财务全貌的方式。",
		"settings.appearance.title":       "外观",
		"settings.appearance.description": "选择适合长期查看资产负债表的明暗模式和强调色。",
		"settings.appearance.mode":        "外观模式",
		"settings.appearance.accent":      "颜色主题",
		"settings.language.title":         "语言与地区",
		"settings.language.description":   "让日期、周起始日和本地时间符合你的工作习惯。",
		"settings.language.language":      "语言",
		"settings.language.timezone":      "时区",
		"settings.language.weekStart":     "每周第一天",
		"settings.language.dateFormat":    "日期格式",
		"settings.language.timeFormat":    "时间格式",
		"settings.numbers.title":          "货币与数字",
		"settings.numbers.description":    "这些选项只影响显示，保存的财务精度始终保持准确。",
		"settings.numbers.currency":       "主显示货币",
		"settings.numbers.decimal":        "小数分隔符",
		"settings.numbers.grouping":       "分组分隔符",
		"settings.numbers.places":         "小数位数",
		"settings.numbers.preview":        "格式预览",
		"settings.reset.title":            "恢复偏好设置？",
		"settings.reset.body":             "所有外观和本地化选项都会恢复默认值。",
		"settings.reset.confirm":          "恢复",
		"settings.saveError":              "本次修改已生效，但无法保存到本地。",
		"option.appearance.system":        "系统",
		"option.appearance.light":         "浅色",
		"option.appearance.dark":          "深色",
		"option.accent.nestworth":         "Nestworth",
		"option.accent.ocean":             "海洋",
		"option.accent.forest":            "森林",
		"option.accent.amber":             "琥珀",
		"option.accent.rose":              "玫瑰",
		"option.language.system":          "系统",
		"option.language.en":              "English",
		"option.language.zhCN":            "简体中文",
		"option.language.zhTW":            "正體中文",
		"option.timezone.system":          "系统时区",
		"option.week.monday":              "星期一",
		"option.week.sunday":              "星期日",
		"option.date.iso":                 "YYYY-MM-DD",
		"option.date.dayFirst":            "DD/MM/YYYY",
		"option.date.monthFirst":          "MM/DD/YYYY",
		"option.date.localized":           "本地化",
		"option.time.24h":                 "24 小时制",
		"option.time.12h":                 "12 小时制",
		"option.grouping.none":            "不分组",
		"option.grouping.space":           "空格",
		"format.sampleLabel":              "示例",
		"format.todayLabel":               "当前本地时间",
	},
	settings.LanguageZhTW: {
		"app.name":                        "Nestworth",
		"menu.file":                       "檔案",
		"menu.about":                      "關於",
		"about.title":                     "關於 Nestworth",
		"about.description":               "一款以本地優先為理念、用於建立和維護個人或家庭資產負債表的財務桌面應用程式。",
		"about.version":                   "版本 %s · 建置 %s",
		"about.license":                   "MIT 授權條款",
		"common.previewData":              "示範資料",
		"common.previewNotice":            "這是視覺預覽。後續版本會在這裡接入你的本地財務資料。",
		"common.comingSoon":               "即將推出",
		"common.openSettings":             "開啟設定",
		"common.reset":                    "恢復預設設定",
		"common.close":                    "關閉",
		"common.saved":                    "已儲存到本地",
		"common.system":                   "系統",
		"common.light":                    "淺色",
		"common.dark":                     "深色",
		"common.invalid":                  "值無效",
		"common.preview":                  "預覽",
		"nav.overview":                    "總覽",
		"nav.accounts":                    "帳戶",
		"nav.activity":                    "活動",
		"nav.analytics":                   "分析",
		"nav.settings":                    "設定",
		"dashboard.eyebrow":               "家庭財務快照",
		"dashboard.title":                 "一處看清你的財務全貌。",
		"dashboard.subtitle":              "以清晰、平靜的方式查看資產負債表，並隨著家庭成長逐步擴展。",
		"dashboard.netWorth":              "淨資產",
		"dashboard.currentTotal":          "目前總額",
		"dashboard.change":                "本月變化",
		"dashboard.changeValue":           "+2.8%",
		"dashboard.assets":                "資產",
		"dashboard.liabilities":           "負債",
		"dashboard.liquidAssets":          "流動資產",
		"dashboard.accountsTracked":       "個帳戶已追蹤",
		"dashboard.netWorthTrend":         "淨資產趨勢",
		"dashboard.previewTrend":          "未來將連接歷史快照，目前僅展示介面預覽。",
		"dashboard.allocation":            "資產配置",
		"dashboard.cash":                  "現金及等價物",
		"dashboard.investments":           "投資",
		"dashboard.property":              "不動產",
		"dashboard.recentActivity":        "近期活動",
		"dashboard.activityDescription":   "活動帳本會解釋這張資產負債表如何隨時間變化。",
		"dashboard.noActivity":            "暫時沒有真實活動",
		"dashboard.accountsToReview":      "待複核帳戶",
		"dashboard.reviewDescription":     "透過輕量的複核節奏，讓資產負債表保持新鮮。",
		"dashboard.reviewItemOne":         "2 個餘額可以更新",
		"dashboard.reviewItemTwo":         "1 個帳戶需要檢查所有權",
		"page.accountsTitle":              "帳戶",
		"page.accountsDescription":        "帳戶將成為管理餘額、所有權、機構和分組的長期入口。",
		"page.activityTitle":              "活動",
		"page.activityDescription":        "活動會解釋存入、轉帳、估值、收入和費用，同時保留清晰的帳本。",
		"page.analyticsTitle":             "分析",
		"page.analyticsDescription":       "分析會區分投入、市場變化、匯率變化、收入和費用。",
		"settings.title":                  "設定",
		"settings.subtitle":               "調整 Nestworth 展示家庭財務全貌的方式。",
		"settings.appearance.title":       "外觀",
		"settings.appearance.description": "選擇適合長期查看資產負債表的明暗模式和強調色。",
		"settings.appearance.mode":        "外觀模式",
		"settings.appearance.accent":      "顏色主題",
		"settings.language.title":         "語言與地區",
		"settings.language.description":   "讓日期、週起始日和本地時間符合你的工作習慣。",
		"settings.language.language":      "語言",
		"settings.language.timezone":      "時區",
		"settings.language.weekStart":     "每週第一天",
		"settings.language.dateFormat":    "日期格式",
		"settings.language.timeFormat":    "時間格式",
		"settings.numbers.title":          "貨幣與數字",
		"settings.numbers.description":    "這些選項只影響顯示，儲存的財務精度始終保持準確。",
		"settings.numbers.currency":       "主顯示貨幣",
		"settings.numbers.decimal":        "小數分隔符",
		"settings.numbers.grouping":       "分組分隔符",
		"settings.numbers.places":         "小數位數",
		"settings.numbers.preview":        "格式預覽",
		"settings.reset.title":            "恢復偏好設定？",
		"settings.reset.body":             "所有外觀和本地化選項都會恢復預設值。",
		"settings.reset.confirm":          "恢復",
		"settings.saveError":              "本次修改已生效，但無法儲存到本地。",
		"option.appearance.system":        "系統",
		"option.appearance.light":         "淺色",
		"option.appearance.dark":          "深色",
		"option.accent.nestworth":         "Nestworth",
		"option.accent.ocean":             "海洋",
		"option.accent.forest":            "森林",
		"option.accent.amber":             "琥珀",
		"option.accent.rose":              "玫瑰",
		"option.language.system":          "系統",
		"option.language.en":              "English",
		"option.language.zhCN":            "簡體中文",
		"option.language.zhTW":            "正體中文",
		"option.timezone.system":          "系統時區",
		"option.week.monday":              "星期一",
		"option.week.sunday":              "星期日",
		"option.date.iso":                 "YYYY-MM-DD",
		"option.date.dayFirst":            "DD/MM/YYYY",
		"option.date.monthFirst":          "MM/DD/YYYY",
		"option.date.localized":           "本地化",
		"option.time.24h":                 "24 小時制",
		"option.time.12h":                 "12 小時制",
		"option.grouping.none":            "不分組",
		"option.grouping.space":           "空格",
		"format.sampleLabel":              "範例",
		"format.todayLabel":               "目前本地時間",
	},
}
