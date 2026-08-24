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

// languageFromLocale parses the language and each following BCP47 subtag so
// script tags win over region defaults: zh-Hant-TW must resolve to
// Traditional Chinese instead of falling into the generic "zh-" bucket.
func languageFromLocale(value string) settings.Language {
	value = strings.ToLower(strings.TrimSpace(strings.Split(value, ":")[0]))
	if cut := strings.IndexAny(value, ".@"); cut >= 0 {
		value = value[:cut]
	}
	subtags := strings.Split(strings.ReplaceAll(value, "_", "-"), "-")
	if subtags[0] != "zh" {
		return settings.LanguageEnglish
	}
	for _, subtag := range subtags[1:] {
		switch subtag {
		case "hant", "tw", "hk", "mo":
			return settings.LanguageZhTW
		case "hans", "cn", "sg":
			return settings.LanguageZhCN
		}
	}
	return settings.LanguageZhCN
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

// translation is one catalog entry in every supported language.
type translation struct {
	english     string
	simplified  string
	traditional string
}

// translations is the single home of every UI catalog key. Each key is owned
// by exactly one entry: duplicates inside this literal are a compile error,
// and a key claimed by two tables panics at assembly time (see buildCatalogs).
// Version-patch files that overwrite earlier entries are gone on purpose —
// add new keys here instead.
var translations = map[string]translation{
	"about.description":                         {"A local-first personal finance desktop application for building and maintaining a personal or household balance sheet.", "一款以本地优先为理念、用于建立和维护个人或家庭资产负债表的财务桌面应用。", "一款以本地優先為理念、用於建立和維護個人或家庭資產負債表的財務桌面應用程式。"},
	"about.license":                             {"MIT License", "MIT 许可证", "MIT 授權條款"},
	"about.title":                               {"About Nestworth", "关于 Nestworth", "關於 Nestworth"},
	"about.version":                             {"Version %s · Build %s", "版本 %s · 构建 %s", "版本 %s · 建置 %s"},
	"accounts.amount":                           {"Initial value", "初始金额", "初始金額"},
	"accounts.amountNotUsed":                    {"Not used for Holdings", "持仓模式不使用", "持倉模式不使用"},
	"accounts.archive":                          {"Archive", "归档", "封存"},
	"accounts.category":                         {"Category", "类别", "類別"},
	"accounts.closedOn":                         {"Closed on", "关闭日期", "关闭日期"},
	"accounts.create":                           {"Add account", "添加账户", "新增帳戶"},
	"accounts.createDescription":                {"Record a current balance or manual value without losing exact ownership.", "记录当前余额或手工估值，并保留精确所有权。", "記錄目前餘額或手工估值，並保留精確所有權。"},
	"accounts.createTitle":                      {"Add an account", "添加账户", "新增帳戶"},
	"accounts.currency":                         {"Currency", "货币", "貨幣"},
	"accounts.datePlaceholder":                  {"Select a date", "选择日期", "選擇日期"},
	"accounts.details":                          {"Details", "详情", "詳情"},
	"accounts.edit":                             {"Edit account", "编辑账户", "编辑账户"},
	"accounts.effectiveDate":                    {"Effective date", "生效日期", "生效日期"},
	"accounts.emptyDescription":                 {"Use Add account to record an asset or liability.", "点击“添加账户”记录资产或负债。", "使用上方表單新增資產或負債。"},
	"accounts.emptyTitle":                       {"No accounts yet", "还没有账户", "還沒有帳戶"},
	"accounts.filter":                           {"Filter", "筛选", "篩選"},
	"accounts.filterAll":                        {"All accounts", "全部账户", "全部帳戶"},
	"accounts.filterShared":                     {"Shared", "共享", "共享"},
	"accounts.filterSole":                       {"Sole-owned", "单独所有", "单独所有"},
	"accounts.includeInInvestment":              {"Include in investment", "计入投资", "计入投资"},
	"accounts.includeInLiquidAssets":            {"Include in liquid assets", "计入流动资产", "计入流动资产"},
	"accounts.includeInNetWorth":                {"Include in net worth", "计入净资产", "计入净资产"},
	"accounts.loadError":                        {"Accounts could not be loaded", "无法加载账户", "無法載入帳戶"},
	"accounts.name":                             {"Name", "名称", "名稱"},
	"accounts.noValue":                          {"No current value", "没有当前值", "沒有目前值"},
	"accounts.none":                             {"Unassigned", "未分配", "未分配"},
	"accounts.note":                             {"Note", "备注", "备注"},
	"accounts.openedOn":                         {"Opened on", "开户日期", "开户日期"},
	"accounts.owner":                            {"Owner", "所有人", "所有人"},
	"accounts.ownershipHint":                    {"Check one or more owners. Leave every percentage blank to split evenly, or fill in every checked owner's percentage so they total 100%.", "勾选一位或多位所有人。所有比例留空将自动平均分配；如需自定义，请为每位勾选的所有人都填写比例，且总和为 100%。", "勾選一位或多位所有人。所有比例留空將自動平均分配；如需自訂，請為每位勾選的所有人都填寫比例，且總和為 100%。"},
	"accounts.ownershipScope":                   {"Ownership", "所有权", "所有权"},
	"accounts.ownershipSharePlaceholder":        {"Optional — split evenly if blank", "可选，留空则自动平均分配", "可選，留空則自動平均分配"},
	"accounts.ownershipShares":                  {"Ownership shares", "所有权比例", "所有权比例"},
	"accounts.secondaryCategory":                {"Subcategory", "子类别", "子類別"},
	"accounts.showArchived":                     {"Show archived", "显示已归档", "顯示已封存"},
	"accounts.trackingMode":                     {"Tracking mode", "跟踪模式", "追蹤模式"},
	"accounts.updateValue":                      {"Update current value", "更新当前值", "更新目前值"},
	"analytics.empty":                           {"No closed-day snapshots are available yet.", "暂时没有可用的已收盘日快照。", "暫時沒有可用的已收盤日快照。"},
	"analytics.range":                           {"Range", "范围", "範圍"},
	"analytics.range1year":                      {"1 year", "1 年", "1 年"},
	"analytics.range30":                         {"30 days", "30 天", "30 天"},
	"analytics.rangeAll":                        {"All history", "全部历史", "全部歷史"},
	"analytics.statusComplete":                  {"Complete", "完整", "完整"},
	"analytics.statusPartial":                   {"Incomplete", "不完整", "不完整"},
	"analytics.trend":                           {"Net-worth trend", "净资产趋势", "淨資產趨勢"},
	"app.name":                                  {"Nestworth", "Nestworth", "Nestworth"},
	"common.active":                             {"Active", "启用", "啟用"},
	"common.add":                                {"Add", "添加", "新增"},
	"common.archive":                            {"Archive", "归档", "封存"},
	"common.archived":                           {"Archived", "已归档", "已封存"},
	"common.cancel":                             {"Cancel", "取消", "取消"},
	"common.chooseIcon":                         {"Choose icon", "选择图标", "選擇圖示"},
	"common.close":                              {"Close", "关闭", "關閉"},
	"common.comingSoon":                         {"Coming soon", "即将推出", "即將推出"},
	"common.dark":                               {"Dark", "深色", "深色"},
	"common.edit":                               {"Edit", "编辑", "編輯"},
	"common.icon":                               {"Icon", "图标", "圖示"},
	"common.invalid":                            {"Invalid value", "值无效", "值無效"},
	"common.light":                              {"Light", "浅色", "淺色"},
	"common.media":                              {"Set image", "设置图片", "設定圖片"},
	"common.openSettings":                       {"Open settings", "打开设置", "開啟設定"},
	"common.pending":                            {"Saving…", "保存中…", "儲存中…"},
	"common.preview":                            {"Preview", "预览", "預覽"},
	"common.previewData":                        {"Preview data", "演示数据", "示範資料"},
	"common.previewNotice":                      {"This is a visual preview. Your local financial data will connect here in a later release.", "这是视觉预览。后续版本会在这里接入你的本地财务数据。", "這是視覺預覽。後續版本會在這裡接入你的本地財務資料。"},
	"common.reset":                              {"Reset to defaults", "恢复默认设置", "恢復預設設定"},
	"common.restore":                            {"Restore", "恢复", "還原"},
	"common.retry":                              {"Retry", "重试", "重試"},
	"common.save":                               {"Save", "保存", "儲存"},
	"common.saved":                              {"Saved locally", "已保存到本地", "已儲存到本地"},
	"common.system":                             {"System", "系统", "系統"},
	"dashboard.accountCount":                    {"%d accounts", "%d 个账户", "%d 個帳戶"},
	"dashboard.accountsToReview":                {"Accounts to review", "待复核账户", "待複核帳戶"},
	"dashboard.accountsTracked":                 {"accounts tracked", "个账户已跟踪", "個帳戶已追蹤"},
	"dashboard.activityDescription":             {"The activity ledger will explain how this picture changes over time.", "活动账本会解释这张资产负债表如何随时间变化。", "活動帳本會解釋這張資產負債表如何隨時間變化。"},
	"dashboard.allocation":                      {"Allocation", "资产分配", "資產配置"},
	"dashboard.assets":                          {"Assets", "资产", "資產"},
	"dashboard.cash":                            {"Cash & equivalents", "现金及等价物", "現金及等價物"},
	"dashboard.change":                          {"This month", "本月变化", "本月變化"},
	"dashboard.changeValue":                     {"+2.8%", "+2.8%", "+2.8%"},
	"dashboard.currentTotal":                    {"Current total", "当前总额", "目前總額"},
	"dashboard.eyebrow":                         {"HOUSEHOLD SNAPSHOT", "家庭资产快照", "家庭財務快照"},
	"dashboard.investments":                     {"Investments", "投资", "投資"},
	"dashboard.liabilities":                     {"Liabilities", "负债", "負債"},
	"dashboard.liquidAssets":                    {"Liquid assets", "流动资产", "流動資產"},
	"dashboard.netWorth":                        {"Net worth", "净资产", "淨資產"},
	"dashboard.netWorthTrend":                   {"Net worth trend", "净资产趋势", "淨資產趨勢"},
	"dashboard.noActivity":                      {"No live activities yet", "暂时没有真实活动", "暫時沒有真實活動"},
	"dashboard.previewTrend":                    {"A lightweight preview of the view that will connect to historical snapshots.", "未来将连接历史快照，目前仅展示界面预览。", "未來將連接歷史快照，目前僅展示介面預覽。"},
	"dashboard.property":                        {"Property", "不动产", "不動產"},
	"dashboard.recentActivity":                  {"Recent activity", "近期活动", "近期活動"},
	"dashboard.reviewDescription":               {"Keep your balance sheet fresh with a quick review rhythm.", "通过轻量的复核节奏，让资产负债表保持新鲜。", "透過輕量的複核節奏，讓資產負債表保持新鮮。"},
	"dashboard.reviewItemOne":                   {"2 balances are ready for a refresh", "2 个余额可以更新", "2 個餘額可以更新"},
	"dashboard.reviewItemTwo":                   {"1 account needs an ownership check", "1 个账户需要检查所有权", "1 個帳戶需要檢查所有權"},
	"dashboard.subtitle":                        {"A calm view of the balance sheet, designed to stay understandable as your household grows.", "以清晰、平静的方式查看资产负债表，并随着家庭成长逐步扩展。", "以清晰、平靜的方式查看資產負債表，並隨著家庭成長逐步擴展。"},
	"dashboard.title":                           {"Your financial picture, in one place.", "一处看清你的财务全貌。", "一處看清你的財務全貌。"},
	"enum.auto_loan":                            {"Auto loan", "车贷", "車貸"},
	"enum.balance":                              {"Balance", "余额", "餘額"},
	"enum.bank_account":                         {"Bank account", "银行账户", "銀行帳戶"},
	"enum.bank_investment_product":              {"Bank investment product", "银行理财产品", "銀行投資產品"},
	"enum.bond":                                 {"Bond", "债券", "債券"},
	"enum.broker_cash":                          {"Broker cash", "券商现金", "券商現金"},
	"enum.brokerage_account":                    {"Brokerage account", "券商账户", "券商帳戶"},
	"enum.cash":                                 {"Cash", "现金", "現金"},
	"enum.cash_equivalent":                      {"Cash & equivalents", "现金及现金等价物", "現金及現金等價物"},
	"enum.collectible":                          {"Collectible", "收藏品", "收藏品"},
	"enum.consumer_loan":                        {"Consumer loan", "消费贷款", "消費貸款"},
	"enum.credit_card":                          {"Credit card", "信用卡", "信用卡"},
	"enum.crypto":                               {"Crypto", "加密资产", "加密資產"},
	"enum.digital_wallet":                       {"Digital wallet", "数字钱包", "數位錢包"},
	"enum.etf":                                  {"ETF", "ETF", "ETF"},
	"enum.holdings":                             {"Holdings", "持仓", "持倉"},
	"enum.insurance":                            {"Insurance", "保险", "保險"},
	"enum.investment":                           {"Investment", "投资", "投資"},
	"enum.investment_fund_account":              {"Investment fund account", "投资基金账户", "投資基金帳戶"},
	"enum.liability":                            {"Liability", "负债", "負債"},
	"enum.loan_receivable":                      {"Loan receivable", "应收贷款", "應收貸款"},
	"enum.manual_investment":                    {"Manual investment", "手工投资", "手工投資"},
	"enum.manual_value":                         {"Manual value", "手工估值", "手工估值"},
	"enum.mortgage":                             {"Mortgage", "按揭", "按揭"},
	"enum.mutual_fund":                          {"Mutual fund", "共同基金", "共同基金"},
	"enum.other":                                {"Other", "其他", "其他"},
	"enum.other_cash_equivalent":                {"Other cash equivalent", "其他现金等价物", "其他現金等價物"},
	"enum.other_investment":                     {"Other investment", "其他投资", "其他投資"},
	"enum.other_liability":                      {"Other liability", "其他负债", "其他負債"},
	"enum.other_property":                       {"Other property", "其他财产", "其他財產"},
	"enum.other_receivable":                     {"Other receivable", "其他应收款", "其他應收款"},
	"enum.personal_debt":                        {"Personal debt", "个人债务", "個人債務"},
	"enum.precious_metal":                       {"Precious metal", "贵金属", "貴金屬"},
	"enum.property":                             {"Property", "房产及其他财产", "房產及其他財產"},
	"enum.real_estate":                          {"Real estate", "房地产", "房地產"},
	"enum.receivable":                           {"Receivable", "应收款", "應收款"},
	"enum.stock":                                {"Stock", "股票", "股票"},
	"enum.vehicle":                              {"Vehicle", "车辆", "車輛"},
	"format.sampleLabel":                        {"Example", "示例", "範例"},
	"format.todayLabel":                         {"Current local time", "当前本地时间", "目前本地時間"},
	"groups.createDescription":                  {"Organize accounts by a household-defined purpose.", "按家庭自定义用途组织账户。", "按家庭自訂用途組織帳戶。"},
	"groups.createTitle":                        {"Add a group", "添加分组", "新增分組"},
	"groups.loadError":                          {"Groups could not be loaded", "无法加载分组", "無法載入分組"},
	"groups.name":                               {"Name", "名称", "名稱"},
	"history.account":                           {"Account", "账户", "帳戶"},
	"history.amount":                            {"Amount", "金额", "金額"},
	"history.applyFilters":                      {"Apply", "应用", "套用"},
	"history.destination":                       {"Destination", "目标账户", "目標帳戶"},
	"history.destinationHolding":                {"Destination holding", "目标持仓", "目標持倉"},
	"history.effectiveDate":                     {"Local date", "本地日期", "本地日期"},
	"history.effectiveTime":                     {"Local time", "本地时间", "本地時間"},
	"history.empty":                             {"No changes have been recorded yet.", "还没有记录任何变更。", "還沒有記錄任何變更。"},
	"history.fee":                               {"Fee", "费用", "費用"},
	"history.filterAccount":                     {"Account", "账户", "帳戶"},
	"history.filterAll":                         {"All", "全部", "全部"},
	"history.filterType":                        {"Type", "类型", "類型"},
	"history.fix":                               {"Fix", "修正", "修正"},
	"history.fixNoChange":                       {"Change at least one value before saving the correction.", "请至少修改一项后再保存修正。", "請至少修改一項後再儲存修正。"},
	"history.fixUnavailable":                    {"This change needs a new correction from the form.", "请从上方表单重新录入这项修正。", "請從上方表單重新輸入這項修正。"},
	"history.fromDate":                          {"From", "起始", "起始"},
	"history.holding":                           {"Holding", "持仓", "持倉"},
	"history.kind.buy":                          {"Buy", "买入", "買入"},
	"history.kind.cash_in":                      {"Money added", "增加金额", "增加金額"},
	"history.kind.cash_out":                     {"Money removed", "减少金额", "減少金額"},
	"history.kind.cash_transfer":                {"Cash transfer", "现金转账", "現金轉帳"},
	"history.kind.debt":                         {"Debt draw / payment", "借款 / 还款", "借款 / 還款"},
	"history.kind.debt_draw":                    {"Debt draw", "借款", "借款"},
	"history.kind.debt_payment":                 {"Debt payment", "还款", "還款"},
	"history.kind.money_added":                  {"Money added", "增加金额", "增加金額"},
	"history.kind.money_removed":                {"Money removed", "减少金额", "減少金額"},
	"history.kind.position_transfer":            {"Investment transfer", "投资转移", "投資轉移"},
	"history.kind.sell":                         {"Sell", "卖出", "賣出"},
	"history.kind.trade":                        {"Buy / sell", "买入 / 卖出", "買入 / 賣出"},
	"history.kind.value_update":                 {"Value update", "更新价值", "更新價值"},
	"history.loadMore":                          {"Load more", "加载更多", "載入更多"},
	"history.note":                              {"Note", "备注", "備註"},
	"history.notePlaceholder":                   {"Optional note", "可选备注", "可選備註"},
	"history.preview":                           {"Preview", "预览", "預覽"},
	"history.preview.cashTransfer":              {"Transfer %s", "转账 %s", "轉帳 %s"},
	"history.preview.change":                    {"Change: %s", "变更：%s", "變更：%s"},
	"history.preview.debtDraw":                  {"Draw debt %s", "借款 %s", "借款 %s"},
	"history.preview.debtPayment":               {"Pay debt %s", "还款 %s", "還款 %s"},
	"history.preview.moneyAdded":                {"Add %s", "增加 %s", "增加 %s"},
	"history.preview.moneyRemoved":              {"Remove %s", "减少 %s", "減少 %s"},
	"history.preview.positionTransfer":          {"Transfer position %s", "转移持仓 %s", "轉移持倉 %s"},
	"history.preview.trade":                     {"Trade %s", "交易 %s", "交易 %s"},
	"history.preview.valueUpdate":               {"Set value to %s", "将价值设为 %s", "將價值設為 %s"},
	"history.quantity":                          {"Quantity", "数量", "數量"},
	"history.reason":                            {"Reason", "原因", "原因"},
	"history.reason.contribution":               {"Contribution", "投入", "投入"},
	"history.reason.expense":                    {"Expense", "支出", "支出"},
	"history.reason.fee":                        {"Fee", "费用", "費用"},
	"history.reason.gift":                       {"Gift", "赠与", "贈與"},
	"history.reason.income":                     {"Income", "收入", "收入"},
	"history.reason.interest":                   {"Interest", "利息", "利息"},
	"history.reason.other":                      {"Other", "其他", "其他"},
	"history.reason.principal":                  {"Principal", "本金", "本金"},
	"history.reason.reconciliation":             {"Reconciliation", "对账", "對帳"},
	"history.reason.tax":                        {"Tax", "税费", "稅費"},
	"history.receivedAmount":                    {"Received amount", "收款金额", "收款金額"},
	"history.recordTitle":                       {"Record a change", "记录变更", "記錄變更"},
	"history.secondaryAmount":                   {"Fee / received amount", "费用 / 收款金额", "費用 / 收款金額"},
	"history.selectAccount":                     {"Select an Account", "请选择账户", "請選擇帳戶"},
	"history.selectHolding":                     {"Select a Holding", "请选择持仓", "請選擇持倉"},
	"history.selectType":                        {"Select a change type", "请选择变更类型", "請選擇變更類型"},
	"history.side":                              {"Action", "操作", "操作"},
	"history.side.buy":                          {"Buy", "买入", "買入"},
	"history.side.draw":                         {"Draw", "借款", "借款"},
	"history.side.payment":                      {"Payment", "还款", "還款"},
	"history.side.sell":                         {"Sell", "卖出", "賣出"},
	"history.start":                             {"Start history", "开始历史记录", "開始歷史記錄"},
	"history.startDescription":                  {"Confirm the household timezone to begin the append-only history.", "确认家庭时区后，才能开始追加式历史记录。", "確認家庭時區後，才能開始追加式歷史記錄。"},
	"history.startingPoint":                     {"Starting point", "起始点", "起始點"},
	"history.timeline":                          {"Timeline", "时间线", "時間線"},
	"history.toDate":                            {"To", "结束", "結束"},
	"history.type":                              {"Change type", "变更类型", "變更類型"},
	"history.undo":                              {"Undo", "撤销", "撤銷"},
	"icons.account":                             {"Account", "账户", "帳戶"},
	"icons.armchair":                            {"Retirement life", "退休生活", "退休生活"},
	"icons.badgeDollar":                         {"Dollar", "美元标记", "美元標記"},
	"icons.badgeEuro":                           {"Euro badge", "欧元标记", "歐元標記"},
	"icons.badgeFranc":                          {"Swiss franc", "瑞士法郎", "瑞士法郎"},
	"icons.badgePercent":                        {"Rate", "比例", "比例"},
	"icons.badgePound":                          {"Pound badge", "英镑标记", "英鎊標記"},
	"icons.badgeRuble":                          {"Russian ruble", "俄罗斯卢布", "俄羅斯盧布"},
	"icons.badgeRupee":                          {"Indian rupee", "印度卢比", "印度盧比"},
	"icons.badgeYen":                            {"Yen badge", "日元标记", "日圓標記"},
	"icons.bank":                                {"Bank", "银行", "銀行"},
	"icons.bankBranch":                          {"Bank branch", "银行网点", "銀行網點"},
	"icons.banknoteDown":                        {"Money out", "支出", "支出"},
	"icons.banknoteUp":                          {"Money in", "收入", "收入"},
	"icons.bitcoin":                             {"Bitcoin", "比特币", "比特幣"},
	"icons.brokerage":                           {"Brokerage", "券商", "券商"},
	"icons.brokerageCash":                       {"Broker cash", "券商现金", "券商現金"},
	"icons.building":                            {"Building", "建筑", "建築"},
	"icons.calendar":                            {"Calendar", "日历", "日曆"},
	"icons.calendarClock":                       {"Schedule", "日程", "日程"},
	"icons.camera":                              {"Camera", "相机", "相機"},
	"icons.card":                                {"Card", "卡片", "卡片"},
	"icons.cash":                                {"Cash", "现金", "現金"},
	"icons.category.accounts":                   {"Accounts & money", "账户与金钱", "帳戶與金錢"},
	"icons.category.banking":                    {"Banking", "银行与机构", "銀行與機構"},
	"icons.category.currency":                   {"Currencies", "货币", "貨幣"},
	"icons.category.general":                    {"General", "通用", "通用"},
	"icons.category.investment":                 {"Investment & markets", "投资与市场", "投資與市場"},
	"icons.category.protection":                 {"Protection & planning", "保障与规划", "保障與規劃"},
	"icons.chart":                               {"Financial chart", "财务图表", "財務圖表"},
	"icons.circleCheck":                         {"Verified", "已确认", "已確認"},
	"icons.circleDollar":                        {"Dollar coin", "美元硬币", "美元硬幣"},
	"icons.circlePercent":                       {"Percentage", "百分率", "百分率"},
	"icons.clock":                               {"Time", "时间", "時間"},
	"icons.coins":                               {"Coins", "硬币", "硬幣"},
	"icons.computer":                            {"Computer", "电脑", "電腦"},
	"icons.creditCard":                          {"Credit card", "信用卡", "信用卡"},
	"icons.currency":                            {"Currency", "货币", "貨幣"},
	"icons.document":                            {"Document", "文档", "文件"},
	"icons.dollar":                              {"US dollar", "美元", "美元"},
	"icons.download":                            {"Download", "下载", "下載"},
	"icons.euro":                                {"Euro", "欧元", "歐元"},
	"icons.file":                                {"File", "文件", "檔案"},
	"icons.folder":                              {"Folder", "文件夹", "資料夾"},
	"icons.goal":                                {"Financial goal", "财务目标", "財務目標"},
	"icons.grid":                                {"Grid", "网格", "網格"},
	"icons.history":                             {"History", "历史", "歷史"},
	"icons.home":                                {"Home", "家庭", "家庭"},
	"icons.info":                                {"Information", "信息", "資訊"},
	"icons.insurance":                           {"Insurance", "保险", "保險"},
	"icons.investment":                          {"Investment", "投资", "投資"},
	"icons.liability":                           {"Liability", "负债", "負債"},
	"icons.list":                                {"List", "列表", "清單"},
	"icons.mail":                                {"Mail", "邮件", "郵件"},
	"icons.market":                              {"Market", "市场", "市場"},
	"icons.media":                               {"Image", "图片", "圖片"},
	"icons.money":                               {"Money", "金钱", "金錢"},
	"icons.pension":                             {"Pension", "养老金", "退休金"},
	"icons.percent":                             {"Percent", "百分比", "百分比"},
	"icons.pound":                               {"Pound sterling", "英镑", "英鎊"},
	"icons.property":                            {"Property", "房产", "房產"},
	"icons.receipt":                             {"Receipt", "收据", "收據"},
	"icons.receivable":                          {"Receivable", "应收款", "應收款"},
	"icons.retirement":                          {"Retirement", "退休", "退休"},
	"icons.savings":                             {"Savings", "储蓄", "儲蓄"},
	"icons.search":                              {"Search", "搜索", "搜尋"},
	"icons.settings":                            {"Settings", "设置", "設定"},
	"icons.shield":                              {"Protection", "保障", "保障"},
	"icons.shieldPlus":                          {"Extended protection", "增强保障", "加強保障"},
	"icons.stock":                               {"Stocks", "股票", "股票"},
	"icons.storage":                             {"Database", "数据库", "資料庫"},
	"icons.trendingUp":                          {"Growth", "增长趋势", "增長趨勢"},
	"icons.upload":                              {"Upload", "上传", "上傳"},
	"icons.vault":                               {"Vault", "金库", "金庫"},
	"icons.visibility":                          {"Visibility", "可见性", "可見性"},
	"icons.wallet":                              {"Wallet", "钱包", "錢包"},
	"icons.walletCards":                         {"Wallet cards", "钱包卡片", "錢包卡片"},
	"icons.warning":                             {"Warning", "警告", "警告"},
	"icons.yen":                                 {"Japanese yen", "日元", "日圓"},
	"institutions.createDescription":            {"Keep track of where accounts are held.", "记录账户所在的位置。", "記錄帳戶所在的位置。"},
	"institutions.createTitle":                  {"Add an institution", "添加机构", "新增機構"},
	"institutions.loadError":                    {"Institutions could not be loaded", "无法加载机构", "無法載入機構"},
	"institutions.name":                         {"Name", "名称", "名稱"},
	"members.createDescription":                 {"Members are used for exact ownership and allocation.", "成员用于精确所有权和分配。", "成員用於精確所有權和分配。"},
	"members.createTitle":                       {"Add a member", "添加成员", "新增成員"},
	"members.loadError":                         {"Members could not be loaded", "无法加载成员", "無法載入成員"},
	"members.name":                              {"Name", "名称", "名稱"},
	"menu.about":                                {"About", "关于", "關於"},
	"menu.file":                                 {"File", "文件", "檔案"},
	"nav.accounts":                              {"Accounts", "账户", "帳戶"},
	"nav.activity":                              {"Activity", "活动", "活動"},
	"nav.analytics":                             {"Analytics", "分析", "分析"},
	"nav.groups":                                {"Groups", "分组", "分組"},
	"nav.history":                               {"History", "历史", "歷史"},
	"nav.institutions":                          {"Institutions", "机构", "機構"},
	"nav.investments":                           {"Investments", "投资", "投資"},
	"nav.members":                               {"Members", "成员", "成員"},
	"nav.overview":                              {"Overview", "概览", "總覽"},
	"nav.settings":                              {"Settings", "设置", "設定"},
	"onboarding.baseCurrency":                   {"Base currency", "基础货币", "基礎貨幣"},
	"onboarding.create":                         {"Create Household", "创建家庭", "建立家庭"},
	"onboarding.description":                    {"Create one local balance sheet and add at least one member.", "创建一个本地资产负债表，并至少添加一名成员。", "建立一個本機資產負債表，並至少新增一名成員。"},
	"onboarding.householdName":                  {"Household name", "家庭名称", "家庭名稱"},
	"onboarding.householdNamePlaceholder":       {"Wang Household", "王氏家庭", "王氏家庭"},
	"onboarding.memberNamesPlaceholder":         {"Alice, Bob", "小王，小李", "小王、小李"},
	"onboarding.members":                        {"Members", "成员", "成員"},
	"onboarding.title":                          {"Start your Household", "开始建立家庭账本", "開始建立家庭帳本"},
	"option.accent.amber":                       {"Amber", "琥珀", "琥珀"},
	"option.accent.forest":                      {"Forest", "森林", "森林"},
	"option.accent.nestworth":                   {"Nestworth", "Nestworth", "Nestworth"},
	"option.accent.ocean":                       {"Ocean", "海洋", "海洋"},
	"option.accent.rose":                        {"Rose", "玫瑰", "玫瑰"},
	"option.appearance.dark":                    {"Dark", "深色", "深色"},
	"option.appearance.light":                   {"Light", "浅色", "淺色"},
	"option.appearance.system":                  {"System", "系统", "系統"},
	"option.date.dayFirst":                      {"DD/MM/YYYY", "DD/MM/YYYY", "DD/MM/YYYY"},
	"option.date.iso":                           {"YYYY-MM-DD", "YYYY-MM-DD", "YYYY-MM-DD"},
	"option.date.localized":                     {"Localized", "本地化", "本地化"},
	"option.date.monthFirst":                    {"MM/DD/YYYY", "MM/DD/YYYY", "MM/DD/YYYY"},
	"option.grouping.none":                      {"None", "不分组", "不分組"},
	"option.grouping.space":                     {"Space", "空格", "空格"},
	"option.language.en":                        {"English", "English", "English"},
	"option.language.system":                    {"System", "系统", "系統"},
	"option.language.zhCN":                      {"简体中文", "简体中文", "簡體中文"},
	"option.language.zhTW":                      {"正體中文", "正體中文", "正體中文"},
	"option.time.12h":                           {"12-hour", "12 小时制", "12 小時制"},
	"option.time.24h":                           {"24-hour", "24 小时制", "24 小時制"},
	"option.timezone.system":                    {"System timezone", "系统时区", "系統時區"},
	"option.week.monday":                        {"Monday", "星期一", "星期一"},
	"option.week.sunday":                        {"Sunday", "星期日", "星期日"},
	"overview.accountCountDetail":               {"active accounts", "个活跃账户", "个活跃账户"},
	"overview.accounts":                         {"Accounts", "账户", "账户"},
	"overview.assets":                           {"Assets", "资产", "資產"},
	"overview.byCategory":                       {"By category", "按类别", "按類別"},
	"overview.byGroup":                          {"By group", "按分组", "按分組"},
	"overview.byInstitution":                    {"By institution", "按机构", "按機構"},
	"overview.byMember":                         {"By member", "按成员", "按成員"},
	"overview.emptyDescription":                 {"Add your first account to see assets, liabilities, and net worth.", "添加第一个账户后，这里会显示资产、负债和净资产。", "新增第一個帳戶後，這裡會顯示資產、負債和淨資產。"},
	"overview.emptyTitle":                       {"Your balance sheet is empty", "资产负债表还是空的", "資產負債表還是空的"},
	"overview.liabilities":                      {"Liabilities", "负债", "負債"},
	"overview.loadError":                        {"Overview could not be loaded", "无法加载概览", "無法載入概覽"},
	"overview.netWorth":                         {"Net worth", "净资产", "淨資產"},
	"page.accountsDescription":                  {"Accounts will become the durable home for balances, ownership, institutions, and groups.", "账户将成为管理余额、所有权、机构和分组的长期入口。", "帳戶將成為管理餘額、所有權、機構和分組的長期入口。"},
	"page.accountsTitle":                        {"Accounts", "账户", "帳戶"},
	"page.activityDescription":                  {"Activities will explain deposits, transfers, valuations, income, and fees without hiding the ledger.", "活动会解释存入、转账、估值、收入和费用，同时保留清晰的账本。", "活動會解釋存入、轉帳、估值、收入和費用，同時保留清晰的帳本。"},
	"page.activityTitle":                        {"Activity", "活动", "活動"},
	"page.analyticsDescription":                 {"Analytics will separate contributions, market movement, currency movement, income, and fees.", "分析会区分投入、市场变化、汇率变化、收入和费用。", "分析會區分投入、市場變化、匯率變化、收入和費用。"},
	"page.analyticsTitle":                       {"Analytics", "分析", "分析"},
	"page.groupsDescription":                    {"Flexible household organization for accounts.", "按家庭自定义维度组织账户。", "按家庭自訂維度組織帳戶。"},
	"page.groupsTitle":                          {"Groups", "分组", "分組"},
	"page.historyDescription":                   {"Preview and record changes while keeping an append-only timeline of financial evidence.", "预览并记录变更，同时保留不可变的财务证据时间线。", "預覽並記錄變更，同時保留不可變的財務證據時間線。"},
	"page.historyTitle":                         {"History", "历史", "歷史"},
	"page.institutionsDescription":              {"Places where accounts are held.", "账户所在的银行、券商或其他机构。", "帳戶所在的銀行、券商或其他機構。"},
	"page.institutionsTitle":                    {"Institutions", "机构", "機構"},
	"page.investmentsDescription":               {"Review current portfolio value, positions, and allocation from saved local observations.", "根据已保存的本地观测查看当前投资组合价值、持仓和配置。", "根據已儲存的本地觀測查看目前投資組合價值、持倉和配置。"},
	"page.investmentsTitle":                     {"Investments", "投资", "投資"},
	"page.membersDescription":                   {"People used for exact account ownership.", "用于精确记录账户所有权的家庭成员。", "用於精確記錄帳戶所有權的家庭成員。"},
	"page.membersTitle":                         {"Members", "成员", "成員"},
	"portfolio.accountDetailTitle":              {"Account details", "账户详情", "帳戶詳情"},
	"portfolio.accounts":                        {"Accounts", "账户", "帳戶"},
	"portfolio.accountsDescription":             {"Investment accounts and their current valued totals.", "投资账户及其当前估值总额。", "投資帳戶及其目前估值總額。"},
	"portfolio.addCash":                         {"Add cash", "添加现金", "新增現金"},
	"portfolio.addHolding":                      {"Add holding", "添加持仓", "新增持倉"},
	"portfolio.addInstrument":                   {"Add instrument", "添加投资标的", "新增投資標的"},
	"portfolio.allocations":                     {"Allocations", "配置", "配置"},
	"portfolio.allocationsDescription":          {"Current value by currency, country, and instrument type.", "按货币、国家和标的类型查看当前价值。", "按貨幣、國家和標的類型查看目前價值。"},
	"portfolio.amount":                          {"Amount", "金额", "金額"},
	"portfolio.archiveHolding":                  {"Archive holding", "归档持仓", "封存持倉"},
	"portfolio.archiveInstrument":               {"Archive instrument", "归档投资标的", "封存投資標的"},
	"portfolio.baseValue":                       {"Base value", "基础货币价值", "基礎貨幣價值"},
	"portfolio.byCountry":                       {"By country", "按国家", "按國家"},
	"portfolio.byCurrency":                      {"By currency", "按货币", "按貨幣"},
	"portfolio.byInstrumentType":                {"By instrument type", "按标的类型", "按標的類型"},
	"portfolio.cash":                            {"Cash", "现金", "現金"},
	"portfolio.cashDescription":                 {"Latest cash observation by currency.", "按货币显示最新现金观测。", "按貨幣顯示最新現金觀測。"},
	"portfolio.complete":                        {"Complete", "完整", "完整"},
	"portfolio.completeness":                    {"Completeness", "完整性", "完整性"},
	"portfolio.completenessDescription":         {"Missing inputs remain visible and are never treated as zero.", "缺少的输入会持续显示，绝不会按零处理。", "缺少的輸入會持續顯示，絕不按零處理。"},
	"portfolio.countryCode":                     {"Country code", "国家代码", "國家代碼"},
	"portfolio.currentValue":                    {"Current value", "当前价值", "目前價值"},
	"portfolio.currentValueDescription":         {"Native and Household-base values from persisted observations.", "来自已保存观测的原币和家庭基础货币价值。", "來自已儲存觀測的原幣和家庭基礎貨幣價值。"},
	"portfolio.delayed":                         {"Delayed observation", "延迟观测", "延遲觀測"},
	"portfolio.editInstrument":                  {"Edit instrument", "编辑投资标的", "編輯投資標的"},
	"portfolio.editQuantity":                    {"Edit quantity", "编辑数量", "編輯數量"},
	"portfolio.frankfurterDisclaimer":           {"Frankfurter is an independent, unaffiliated source. Rates are daily and may change or become unavailable; check the provider before relying on a quote.", "Frankfurter 是独立且无关联关系的数据来源。汇率按日更新，可能变化或变得不可用；使用报价前请查看提供方状态。", "Frankfurter 是獨立且無關聯關係的資料來源。匯率按日更新，可能變更或變得不可用；使用報價前請查看提供方狀態。"},
	"portfolio.freshness.delayed":               {"Delayed", "延迟", "延遲"},
	"portfolio.freshness.fresh":                 {"Fresh", "新鲜", "新鮮"},
	"portfolio.freshness.manual":                {"Manual", "手工", "手工"},
	"portfolio.freshness.stale":                 {"Stale", "过期", "過期"},
	"portfolio.freshness.unavailable":           {"Unavailable", "不可用", "不可用"},
	"portfolio.fxDirection":                     {"Direction", "方向", "方向"},
	"portfolio.fxEvidence":                      {"FX", "汇率", "匯率"},
	"portfolio.fxOrientationExplanation":        {"The rate means 1 base currency equals rate quote currency. You may enter either direction.", "汇率含义为 1 单位基础货币等于指定数量的报价货币；可以输入任一方向。", "匯率含義為 1 單位基礎貨幣等於指定數量的報價貨幣；可以輸入任一方向。"},
	"portfolio.fxRates":                         {"FX rates", "汇率", "匯率"},
	"portfolio.fxRatesDescription":              {"Enter a required direct or inverse rate against the Household base currency.", "输入相对于家庭基础货币的直接或逆向汇率。", "輸入相對於家庭基礎貨幣的直接或逆向匯率。"},
	"portfolio.holdings":                        {"Holdings", "持仓", "持倉"},
	"portfolio.holdingsDescription":             {"Current quantities and their selected quote evidence.", "当前数量及其选定的报价证据。", "目前數量及其選定的報價證據。"},
	"portfolio.identityConversion":              {"No FX conversion required", "无需汇率换算", "無需匯率換算"},
	"portfolio.incomplete":                      {"Incomplete", "信息不完整", "資訊不完整"},
	"portfolio.instrument":                      {"Instrument", "标的", "標的"},
	"portfolio.instrumentName":                  {"Instrument name", "标的名称", "標的名稱"},
	"portfolio.instrumentType":                  {"Instrument type", "标的类型", "標的類型"},
	"portfolio.instruments":                     {"Instruments", "投资标的", "投資標的"},
	"portfolio.instrumentsDescription":          {"Maintain reusable instruments and their manual current prices.", "管理可复用的投资标的及其手工当前价格。", "管理可重用的投資標的及其手工目前價格。"},
	"portfolio.isin":                            {"ISIN", "ISIN", "ISIN"},
	"portfolio.loadError":                       {"Unable to load Investments", "无法加载投资页面", "無法載入投資頁面"},
	"portfolio.manualPrice":                     {"Manual price", "手工价格", "手工價格"},
	"portfolio.marketCode":                      {"Market code", "市场代码", "市場代碼"},
	"portfolio.missingAccountValue":             {"Missing account value", "缺少账户价值", "缺少帳戶價值"},
	"portfolio.missingFXRate":                   {"Missing FX rate", "缺少汇率", "缺少匯率"},
	"portfolio.missingInputs":                   {"Missing inputs", "缺少输入", "缺少輸入"},
	"portfolio.missingInstrumentPrice":          {"Missing instrument price", "缺少标的价格", "缺少標的價格"},
	"portfolio.nativeValue":                     {"Native value", "原币价值", "原幣價值"},
	"portfolio.noAllocations":                   {"No allocations", "没有配置数据", "沒有配置資料"},
	"portfolio.noAllocationsDescription":        {"Complete valued positions will appear here.", "完整的已估值持仓会显示在此处。", "完整的已估值持倉會顯示在此處。"},
	"portfolio.noCash":                          {"No cash observations", "还没有现金观测", "還沒有現金觀測"},
	"portfolio.noCashDescription":               {"Add a cash balance when the account holds uninvested money.", "账户有未投资现金时，可以添加现金余额。", "帳戶有未投資現金時，可以新增現金餘額。"},
	"portfolio.noEvidence":                      {"No quote evidence", "没有报价证据", "沒有報價證據"},
	"portfolio.noHoldings":                      {"No holdings yet", "还没有持仓", "還沒有持倉"},
	"portfolio.noHoldingsDescription":           {"Add a holding to start valuing this account.", "添加持仓后即可开始估值。", "新增持倉後即可開始估值。"},
	"portfolio.noInstruments":                   {"No instruments yet", "还没有投资标的", "還沒有投資標的"},
	"portfolio.noInstrumentsDescription":        {"Create an instrument before adding a holding.", "添加持仓前请先创建投资标的。", "新增持倉前請先建立投資標的。"},
	"portfolio.noInstrumentsForHolding":         {"Create an instrument before adding a holding.", "添加持仓前请先创建投资标的。", "新增持倉前請先建立投資標的。"},
	"portfolio.noInvestmentAccounts":            {"No investment accounts", "没有投资账户", "沒有投資帳戶"},
	"portfolio.noInvestmentAccountsDescription": {"Accounts included in Investments appear here after they have a current local value.", "纳入投资的账户有当前本地价值后会显示在此处。", "納入投資的帳戶有目前本地價值後會顯示在此處。"},
	"portfolio.noMissingInputs":                 {"No missing inputs", "没有缺少的输入", "沒有缺少的輸入"},
	"portfolio.noPositions":                     {"No positions yet", "还没有持仓", "還沒有持倉"},
	"portfolio.noPositionsDescription":          {"Add an investment account position or cash observation to see it here.", "添加投资账户持仓或现金观测后即可在此查看。", "新增投資帳戶持倉或現金觀測後即可在此查看。"},
	"portfolio.noRequiredFX":                    {"No missing FX rates", "没有缺少的汇率", "沒有缺少的匯率"},
	"portfolio.noRequiredFXDescription":         {"All currently valued components have the required persisted conversion evidence.", "当前估值组件都已有所需的已保存换算证据。", "目前估值元件都已有所需的已儲存換算證據。"},
	"portfolio.note":                            {"Note", "备注", "備註"},
	"portfolio.positions":                       {"Positions", "持仓", "持倉"},
	"portfolio.positionsDescription":            {"Persisted native values and quote evidence for current positions.", "当前持仓的已保存原币价值和报价证据。", "目前持倉的已儲存原幣價值和報價證據。"},
	"portfolio.price":                           {"Unit price", "单位价格", "單位價格"},
	"portfolio.priceEvidence":                   {"Price", "价格", "價格"},
	"portfolio.provider":                        {"Provider", "提供方", "提供方"},
	"portfolio.providerBindingRequired":         {"A Yahoo symbol is required when Provider is selected.", "选择提供方时必须填写 Yahoo 代码。", "選擇提供方時必須填寫 Yahoo 代碼。"},
	"portfolio.providerSymbol":                  {"Yahoo symbol", "Yahoo 代码", "Yahoo 代碼"},
	"portfolio.providerSymbolPlaceholder":       {"For example, QQQ", "例如：QQQ", "例如：QQQ"},
	"portfolio.quantity":                        {"Quantity", "数量", "數量"},
	"portfolio.quoteCurrency":                   {"Quote currency", "报价货币", "報價貨幣"},
	"portfolio.quoteDate":                       {"Quote date", "报价日期", "報價日期"},
	"portfolio.rate":                            {"Rate", "汇率", "匯率"},
	"portfolio.refreshAll":                      {"Refresh all", "刷新全部", "重新整理全部"},
	"portfolio.refreshFX":                       {"Refresh required FX", "刷新所需汇率", "重新整理所需匯率"},
	"portfolio.refreshPrice":                    {"Refresh price", "刷新价格", "重新整理價格"},
	"portfolio.refreshRate":                     {"Refresh rate", "刷新汇率", "重新整理匯率"},
	"portfolio.refreshSummary":                  {"Refresh: %d fetched, %d cached, %d skipped, %d failed, %d rate-limited", "刷新结果：获取 %d，缓存 %d，跳过 %d，失败 %d，受限 %d", "重新整理結果：取得 %d，快取 %d，略過 %d，失敗 %d，受限 %d"},
	"portfolio.refreshing":                      {"Refreshing provider data…", "正在刷新提供方数据…", "正在重新整理提供方資料…"},
	"portfolio.savePrice":                       {"Save price", "保存价格", "儲存價格"},
	"portfolio.saveRate":                        {"Save rate", "保存汇率", "儲存匯率"},
	"portfolio.source":                          {"Source", "来源", "來源"},
	"portfolio.source.manual":                   {"Manual", "手工", "手工"},
	"portfolio.source.provider":                 {"Provider", "提供方", "提供方"},
	"portfolio.symbol":                          {"Symbol", "代码", "代碼"},
	"portfolio.unavailable":                     {"Unavailable", "不可用", "不可用"},
	"portfolio.unknownInstrument":               {"Unknown instrument", "未知标的", "未知標的"},
	"portfolio.valuedSubtotal":                  {"Valued subtotal", "已估值小计", "已估值小計"},
	"portfolio.valuedSubtotalDescription":       {"Complete current values in the Household base currency.", "家庭基础货币中的完整当前价值。", "家庭基礎貨幣中的完整目前價值。"},
	"portfolio.yahooDisclaimer":                 {"Yahoo Finance is an unofficial, unaffiliated source. Data may change or become unavailable.", "Yahoo Finance 是非官方且无关联关系的数据来源，数据可能变化或变得不可用。", "Yahoo Finance 是非官方且無關聯關係的資料來源，資料可能變更或變得不可用。"},
	"portfolio.yahooDisclaimerLabel":            {"Provider notice", "提供方说明", "提供方說明"},
	"portfolio.yahooFinance":                    {"Yahoo Finance", "Yahoo Finance", "Yahoo Finance"},
	"settings.appearance.accent":                {"Color theme", "颜色主题", "顏色主題"},
	"settings.appearance.description":           {"Choose the surface and accent that make the balance sheet comfortable to return to.", "选择适合长期查看资产负债表的明暗模式和强调色。", "選擇適合長期查看資產負債表的明暗模式和強調色。"},
	"settings.appearance.mode":                  {"Appearance", "外观模式", "外觀模式"},
	"settings.appearance.title":                 {"Appearance", "外观", "外觀"},
	"settings.household.baseCurrency":           {"Base currency", "基础货币", "基礎貨幣"},
	"settings.household.description":            {"Set once during onboarding. Read-only here.", "在引导流程中设置一次，此处仅供查看。", "在引導流程中設定一次，此處僅供查看。"},
	"settings.household.name":                   {"Household name", "家庭名称", "家庭名稱"},
	"settings.household.title":                  {"Household", "家庭", "家庭"},
	"settings.language.dateFormat":              {"Date format", "日期格式", "日期格式"},
	"settings.language.description":             {"Keep dates, week boundaries, and local time aligned with how you work.", "让日期、周起始日和本地时间符合你的工作习惯。", "讓日期、週起始日和本地時間符合你的工作習慣。"},
	"settings.language.language":                {"Language", "语言", "語言"},
	"settings.language.timeFormat":              {"Time format", "时间格式", "時間格式"},
	"settings.language.timezone":                {"Timezone", "时区", "時區"},
	"settings.language.title":                   {"Language & region", "语言与地区", "語言與地區"},
	"settings.language.weekStart":               {"First day of week", "每周第一天", "每週第一天"},
	"settings.numbers.currency":                 {"Sample display currency (preview only)", "示例展示货币（仅用于下方预览）", "示例展示貨幣（僅用於下方預覽）"},
	"settings.numbers.decimal":                  {"Decimal separator", "小数分隔符", "小數分隔符"},
	"settings.numbers.description":              {"These choices affect presentation only; stored financial precision remains exact.", "这些选项只影响显示，保存的财务精度始终保持准确。", "這些選項只影響顯示，儲存的財務精度始終保持準確。"},
	"settings.numbers.grouping":                 {"Grouping separator", "分组分隔符", "分組分隔符"},
	"settings.numbers.places":                   {"Decimal places", "小数位数", "小數位數"},
	"settings.numbers.preview":                  {"Formatting preview", "格式预览", "格式預覽"},
	"settings.numbers.title":                    {"Currency & numbers", "货币与数字", "貨幣與數字"},
	"settings.provider.frankfurter":             {"Frankfurter", "Frankfurter", "Frankfurter"},
	"settings.provider.yahoo":                   {"Yahoo Finance", "Yahoo Finance", "Yahoo Finance"},
	"settings.providers.description":            {"Choose the provider used for explicit FX refreshes. Instrument refreshes keep their own saved provider binding.", "选择手动刷新汇率时使用的提供方。投资标的刷新仍使用各自保存的提供方绑定。", "選擇手動重新整理匯率時使用的提供方。投資標的重新整理仍使用各自儲存的提供方繫結。"},
	"settings.providers.fxProvider":             {"FX provider", "汇率提供方", "匯率提供方"},
	"settings.providers.title":                  {"Market-data providers", "市场数据提供方", "市場資料提供方"},
	"settings.reset.body":                       {"All presentation choices and market-data routing will return to their defaults.", "所有显示选项和市场数据路由都会恢复默认值。", "所有顯示選項和市場資料路由都會恢復預設值。"},
	"settings.reset.confirm":                    {"Reset", "恢复", "恢復"},
	"settings.reset.title":                      {"Reset preferences?", "恢复偏好设置？", "恢復偏好設定？"},
	"settings.saveError":                        {"Your change is active for this session, but could not be saved locally.", "本次修改已生效，但无法保存到本地。", "本次修改已生效，但無法儲存到本地。"},
	"settings.subtitle":                         {"Shape the way Nestworth reads and presents your household picture.", "调整 Nestworth 展示家庭财务全貌的方式。", "調整 Nestworth 展示家庭財務全貌的方式。"},
	"settings.title":                            {"Settings", "设置", "設定"},
	"snapshot.historyRebuild":                   {"History rebuild", "重建历史记录", "重建歷史記錄"},
	"snapshot.noHistory":                        {"Start history to build closed-day snapshots.", "开始历史记录后才能建立已收盘日快照。", "開始歷史記錄後才能建立已收盤日快照。"},
	"snapshot.revisionsUpdated":                 {"%d history revisions updated", "已更新 %d 条历史修订", "已更新 %d 條歷史修訂"},
	"snapshot.updating":                         {"Updating history…", "正在更新历史记录…", "正在更新歷史記錄…"},
	"startup.blockedDescription":                {"Nestworth kept financial writes disabled because the local database could not be opened safely.", "由于无法安全打开本地数据库，Nestworth 已禁用财务数据写入。", "由於無法安全開啟本機資料庫，Nestworth 已停用財務資料寫入。"},
	"startup.blockedTitle":                      {"Local database unavailable", "本地数据库不可用", "本機資料庫無法使用"},
	"startup.readOnly":                          {"Business data is read-only", "业务数据只读", "業務資料唯讀"},
}

// errorTranslations lives in errors.go and is folded in here so the whole
// catalog is assembled at a single point, independent of file order.
func init() {
	built, conflicts := buildCatalogs(translations, errorTranslations)
	if len(conflicts) > 0 {
		panic("i18n: duplicate catalog keys written by multiple tables: " + strings.Join(conflicts, ", "))
	}
	catalogs = built
}

// buildCatalogs assembles per-language catalogs and returns the sorted list
// of keys claimed by more than one source table.
func buildCatalogs(groups ...map[string]translation) (map[settings.Language]map[string]string, []string) {
	english := make(map[string]string)
	simplified := make(map[string]string)
	traditional := make(map[string]string)
	var conflicts []string
	for _, group := range groups {
		for key, entry := range group {
			if _, clash := english[key]; clash {
				conflicts = append(conflicts, key)
				continue
			}
			english[key] = entry.english
			simplified[key] = entry.simplified
			traditional[key] = entry.traditional
		}
	}
	sort.Strings(conflicts)
	return map[settings.Language]map[string]string{
		settings.LanguageEnglish: english,
		settings.LanguageZhCN:    simplified,
		settings.LanguageZhTW:    traditional,
	}, conflicts
}

var catalogs map[settings.Language]map[string]string
