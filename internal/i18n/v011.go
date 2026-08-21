package i18n

import "github.com/waltwang/nestworth-go/internal/settings"

func init() {
	catalog := catalogs[settings.LanguageZhCN]
	for key, value := range map[string]string{
		"nav.members": "成员", "nav.institutions": "机构", "nav.groups": "分组",
		"page.membersTitle": "成员", "page.membersDescription": "用于精确记录账户所有权的家庭成员。",
		"page.institutionsTitle": "机构", "page.institutionsDescription": "账户所在的银行、券商或其他机构。",
		"page.groupsTitle": "分组", "page.groupsDescription": "按家庭自定义维度组织账户。",
		"startup.blockedTitle": "本地数据库不可用", "startup.blockedDescription": "由于无法安全打开本地数据库，Nestworth 已禁用财务数据写入。", "startup.readOnly": "业务数据只读",
		"onboarding.title": "开始建立家庭账本", "onboarding.description": "创建一个本地资产负债表，并至少添加一名成员。", "onboarding.householdName": "家庭名称", "onboarding.householdNamePlaceholder": "王氏家庭", "onboarding.baseCurrency": "基础货币", "onboarding.members": "成员", "onboarding.memberNamesPlaceholder": "小王，小李", "onboarding.create": "创建家庭",
		"overview.assets": "资产", "overview.liabilities": "负债", "overview.netWorth": "净资产", "overview.byCategory": "按类别", "overview.byMember": "按成员", "overview.byInstitution": "按机构", "overview.byGroup": "按分组", "overview.emptyTitle": "资产负债表还是空的", "overview.emptyDescription": "添加第一个账户后，这里会显示资产、负债和净资产。", "overview.loadError": "无法加载概览",
		"accounts.filter": "筛选", "accounts.filterAll": "全部账户", "accounts.createTitle": "添加账户", "accounts.createDescription": "记录当前余额或手工估值，并保留精确所有权。", "accounts.create": "添加账户", "accounts.name": "名称", "accounts.amount": "初始金额", "accounts.category": "类别", "accounts.secondaryCategory": "子类别", "accounts.trackingMode": "跟踪模式", "accounts.owner": "所有人", "accounts.emptyTitle": "还没有账户", "accounts.emptyDescription": "使用上方表单添加资产或负债。", "accounts.noValue": "没有当前值", "accounts.archive": "归档", "accounts.loadError": "无法加载账户",
		"members.createTitle": "添加成员", "members.createDescription": "成员用于精确所有权和分配。", "members.name": "名称", "members.loadError": "无法加载成员",
		"institutions.createTitle": "添加机构", "institutions.createDescription": "记录账户所在的位置。", "institutions.name": "名称", "institutions.loadError": "无法加载机构",
		"groups.createTitle": "添加分组", "groups.createDescription": "按家庭自定义用途组织账户。", "groups.name": "名称", "groups.loadError": "无法加载分组",
		"common.add": "添加", "common.archive": "归档", "common.active": "启用", "common.archived": "已归档", "common.media": "设置图片", "common.icon": "图标", "common.chooseIcon": "选择图标", "common.save": "保存", "common.cancel": "取消", "common.restore": "恢复", "accounts.updateValue": "更新当前值", "accounts.effectiveDate": "生效日期", "accounts.none": "未分配", "accounts.showArchived": "显示已归档",
	} {
		catalog[key] = value
		catalogs[settings.LanguageZhTW][key] = value
	}
	catalogs[settings.LanguageEnglish]["common.media"] = "Set image"
	catalogs[settings.LanguageEnglish]["common.icon"] = "Icon"
	catalogs[settings.LanguageEnglish]["common.chooseIcon"] = "Choose icon"
	catalogs[settings.LanguageEnglish]["common.save"] = "Save"
	catalogs[settings.LanguageEnglish]["common.cancel"] = "Cancel"
	catalogs[settings.LanguageEnglish]["accounts.updateValue"] = "Update current value"
	catalogs[settings.LanguageEnglish]["accounts.effectiveDate"] = "Effective date"
	catalogs[settings.LanguageEnglish]["accounts.datePlaceholder"] = "Select a date"
	catalogs[settings.LanguageEnglish]["accounts.emptyDescription"] = "Use Add account to record an asset or liability."
	catalogs[settings.LanguageEnglish]["accounts.none"] = "Unassigned"
	catalogs[settings.LanguageEnglish]["common.restore"] = "Restore"
	catalogs[settings.LanguageEnglish]["accounts.showArchived"] = "Show archived"
	catalogs[settings.LanguageZhCN]["accounts.datePlaceholder"] = "选择日期"
	catalogs[settings.LanguageZhTW]["accounts.datePlaceholder"] = "選擇日期"
	catalogs[settings.LanguageZhCN]["accounts.emptyDescription"] = "点击“添加账户”记录资产或负债。"
	catalogs[settings.LanguageZhTW]["accounts.emptyDescription"] = "點擊「新增帳戶」記錄資產或負債。"
}

func init() {
	english := map[string]string{
		"common.edit": "Edit", "overview.accounts": "Accounts", "overview.accountCountDetail": "active accounts", "accounts.filterSole": "Sole-owned", "accounts.filterShared": "Shared", "accounts.ownershipScope": "Ownership", "accounts.ownershipShares": "Ownership shares", "accounts.includeInNetWorth": "Include in net worth", "accounts.includeInInvestment": "Include in investment", "accounts.includeInLiquidAssets": "Include in liquid assets", "accounts.note": "Note", "accounts.openedOn": "Opened on", "accounts.closedOn": "Closed on", "accounts.edit": "Edit account",
	}
	simplified := map[string]string{
		"common.edit": "编辑", "overview.accounts": "账户", "overview.accountCountDetail": "个活跃账户", "accounts.filterSole": "单独所有", "accounts.filterShared": "共享", "accounts.ownershipScope": "所有权", "accounts.ownershipShares": "所有权比例", "accounts.includeInNetWorth": "计入净资产", "accounts.includeInInvestment": "计入投资", "accounts.includeInLiquidAssets": "计入流动资产", "accounts.note": "备注", "accounts.openedOn": "开户日期", "accounts.closedOn": "关闭日期", "accounts.edit": "编辑账户",
	}
	traditional := map[string]string{
		"nav.members": "成員", "nav.institutions": "機構", "nav.groups": "分組", "page.membersTitle": "成員", "page.membersDescription": "用於精確記錄帳戶所有權的家庭成員。", "page.institutionsTitle": "機構", "page.institutionsDescription": "帳戶所在的銀行、券商或其他機構。", "page.groupsTitle": "分組", "page.groupsDescription": "按家庭自訂維度組織帳戶。", "startup.blockedTitle": "本機資料庫無法使用", "startup.blockedDescription": "由於無法安全開啟本機資料庫，Nestworth 已停用財務資料寫入。", "startup.readOnly": "業務資料唯讀", "onboarding.title": "開始建立家庭帳本", "onboarding.description": "建立一個本機資產負債表，並至少新增一名成員。", "onboarding.householdName": "家庭名稱", "onboarding.householdNamePlaceholder": "王氏家庭", "onboarding.baseCurrency": "基礎貨幣", "onboarding.members": "成員", "onboarding.memberNamesPlaceholder": "小王、小李", "onboarding.create": "建立家庭", "overview.assets": "資產", "overview.liabilities": "負債", "overview.netWorth": "淨資產", "overview.byCategory": "按類別", "overview.byMember": "按成員", "overview.byInstitution": "按機構", "overview.byGroup": "按分組", "overview.emptyTitle": "資產負債表還是空的", "overview.emptyDescription": "新增第一個帳戶後，這裡會顯示資產、負債和淨資產。", "overview.loadError": "無法載入概覽", "accounts.filter": "篩選", "accounts.filterAll": "全部帳戶", "accounts.createTitle": "新增帳戶", "accounts.createDescription": "記錄目前餘額或手工估值，並保留精確所有權。", "accounts.create": "新增帳戶", "accounts.name": "名稱", "accounts.amount": "初始金額", "accounts.category": "類別", "accounts.secondaryCategory": "子類別", "accounts.trackingMode": "追蹤模式", "accounts.owner": "所有人", "accounts.emptyTitle": "還沒有帳戶", "accounts.emptyDescription": "使用上方表單新增資產或負債。", "accounts.noValue": "沒有目前值", "accounts.archive": "封存", "accounts.loadError": "無法載入帳戶", "members.createTitle": "新增成員", "members.createDescription": "成員用於精確所有權和分配。", "members.name": "名稱", "members.loadError": "無法載入成員", "institutions.createTitle": "新增機構", "institutions.createDescription": "記錄帳戶所在的位置。", "institutions.name": "名稱", "institutions.loadError": "無法載入機構", "groups.createTitle": "新增分組", "groups.createDescription": "按家庭自訂用途組織帳戶。", "groups.name": "名稱", "groups.loadError": "無法載入分組", "common.add": "新增", "common.archive": "封存", "common.active": "啟用", "common.archived": "已封存", "common.media": "設定圖片", "common.icon": "圖示", "common.chooseIcon": "選擇圖示", "common.save": "儲存", "common.cancel": "取消", "common.restore": "還原", "common.edit": "編輯", "accounts.updateValue": "更新目前值", "accounts.effectiveDate": "生效日期", "accounts.none": "未分配", "accounts.showArchived": "顯示已封存",
	}
	for key, value := range english {
		catalogs[settings.LanguageEnglish][key] = value
	}
	for key, value := range simplified {
		catalogs[settings.LanguageZhCN][key] = value
	}
	for key, value := range traditional {
		catalogs[settings.LanguageZhTW][key] = value
	}
	for key, value := range simplified {
		if _, ok := catalogs[settings.LanguageZhTW][key]; !ok {
			catalogs[settings.LanguageZhTW][key] = value
		}
	}
}
func init() {
	english := map[string]string{"enum.cash_equivalent": "Cash & equivalents", "enum.investment": "Investment", "enum.property": "Property", "enum.receivable": "Receivable", "enum.liability": "Liability", "enum.balance": "Balance", "enum.manual_value": "Manual value", "enum.cash": "Cash", "enum.bank_account": "Bank account", "enum.digital_wallet": "Digital wallet", "enum.broker_cash": "Broker cash", "enum.other_cash_equivalent": "Other cash equivalent", "enum.brokerage_account": "Brokerage account", "enum.investment_fund_account": "Investment fund account", "enum.bank_investment_product": "Bank investment product", "enum.insurance": "Insurance", "enum.manual_investment": "Manual investment", "enum.other_investment": "Other investment", "enum.real_estate": "Real estate", "enum.vehicle": "Vehicle", "enum.collectible": "Collectible", "enum.other_property": "Other property", "enum.loan_receivable": "Loan receivable", "enum.other_receivable": "Other receivable", "enum.credit_card": "Credit card", "enum.mortgage": "Mortgage", "enum.auto_loan": "Auto loan", "enum.consumer_loan": "Consumer loan", "enum.personal_debt": "Personal debt", "enum.other_liability": "Other liability"}
	simplified := map[string]string{"enum.cash_equivalent": "现金及现金等价物", "enum.investment": "投资", "enum.property": "房产及其他财产", "enum.receivable": "应收款", "enum.liability": "负债", "enum.balance": "余额", "enum.manual_value": "手工估值", "enum.bank_account": "银行账户", "enum.brokerage_account": "券商账户", "enum.real_estate": "房地产", "enum.vehicle": "车辆", "enum.loan_receivable": "应收贷款", "enum.credit_card": "信用卡", "enum.mortgage": "按揭", "enum.auto_loan": "车贷", "enum.consumer_loan": "消费贷款", "enum.personal_debt": "个人债务"}
	traditional := map[string]string{"enum.cash_equivalent": "現金及現金等價物", "enum.investment": "投資", "enum.property": "房產及其他財產", "enum.receivable": "應收款", "enum.liability": "負債", "enum.balance": "餘額", "enum.manual_value": "手工估值", "enum.bank_account": "銀行帳戶", "enum.brokerage_account": "券商帳戶", "enum.real_estate": "房地產", "enum.vehicle": "車輛", "enum.loan_receivable": "應收貸款", "enum.credit_card": "信用卡", "enum.mortgage": "按揭", "enum.auto_loan": "車貸", "enum.consumer_loan": "消費貸款", "enum.personal_debt": "個人債務"}
	for key, value := range english {
		catalogs[settings.LanguageEnglish][key] = value
	}
	for key, value := range simplified {
		catalogs[settings.LanguageZhCN][key] = value
	}
	for key, value := range traditional {
		catalogs[settings.LanguageZhTW][key] = value
	}
	for key, value := range english {
		if catalogs[settings.LanguageZhCN][key] == "" {
			catalogs[settings.LanguageZhCN][key] = value
		}
		if catalogs[settings.LanguageZhTW][key] == "" {
			catalogs[settings.LanguageZhTW][key] = value
		}
	}
}

func init() {
	translations := map[string][3]string{
		"icons.category.banking":    {"Banking", "银行与机构", "銀行與機構"},
		"icons.category.accounts":   {"Accounts & money", "账户与金钱", "帳戶與金錢"},
		"icons.category.investment": {"Investment & markets", "投资与市场", "投資與市場"},
		"icons.category.currency":   {"Currencies", "货币", "貨幣"},
		"icons.category.protection": {"Protection & planning", "保障与规划", "保障與規劃"},
		"icons.category.general":    {"General", "通用", "通用"},
		"icons.bank":                {"Bank", "银行", "銀行"},
		"icons.bankBranch":          {"Bank branch", "银行网点", "銀行網點"},
		"icons.building":            {"Building", "建筑", "建築"},
		"icons.vault":               {"Vault", "金库", "金庫"},
		"icons.storage":             {"Database", "数据库", "資料庫"},
		"icons.account":             {"Account", "账户", "帳戶"},
		"icons.wallet":              {"Wallet", "钱包", "錢包"},
		"icons.walletCards":         {"Wallet cards", "钱包卡片", "錢包卡片"},
		"icons.cash":                {"Cash", "现金", "現金"},
		"icons.coins":               {"Coins", "硬币", "硬幣"},
		"icons.money":               {"Money", "金钱", "金錢"},
		"icons.card":                {"Card", "卡片", "卡片"},
		"icons.creditCard":          {"Credit card", "信用卡", "信用卡"},
		"icons.receipt":             {"Receipt", "收据", "收據"},
		"icons.banknoteUp":          {"Money in", "收入", "收入"},
		"icons.banknoteDown":        {"Money out", "支出", "支出"},
		"icons.brokerage":           {"Brokerage", "券商", "券商"},
		"icons.brokerageCash":       {"Broker cash", "券商现金", "券商現金"},
		"icons.investment":          {"Investment", "投资", "投資"},
		"icons.stock":               {"Stocks", "股票", "股票"},
		"icons.market":              {"Market", "市场", "市場"},
		"icons.chart":               {"Financial chart", "财务图表", "財務圖表"},
		"icons.trendingUp":          {"Growth", "增长趋势", "增長趨勢"},
		"icons.percent":             {"Percent", "百分比", "百分比"},
		"icons.badgePercent":        {"Rate", "比例", "比例"},
		"icons.circlePercent":       {"Percentage", "百分率", "百分率"},
		"icons.currency":            {"Currency", "货币", "貨幣"},
		"icons.dollar":              {"US dollar", "美元", "美元"},
		"icons.badgeDollar":         {"Dollar", "美元标记", "美元標記"},
		"icons.circleDollar":        {"Dollar coin", "美元硬币", "美元硬幣"},
		"icons.euro":                {"Euro", "欧元", "歐元"},
		"icons.badgeEuro":           {"Euro badge", "欧元标记", "歐元標記"},
		"icons.yen":                 {"Japanese yen", "日元", "日圓"},
		"icons.badgeYen":            {"Yen badge", "日元标记", "日圓標記"},
		"icons.pound":               {"Pound sterling", "英镑", "英鎊"},
		"icons.badgePound":          {"Pound badge", "英镑标记", "英鎊標記"},
		"icons.badgeRupee":          {"Indian rupee", "印度卢比", "印度盧比"},
		"icons.badgeRuble":          {"Russian ruble", "俄罗斯卢布", "俄羅斯盧布"},
		"icons.badgeFranc":          {"Swiss franc", "瑞士法郎", "瑞士法郎"},
		"icons.bitcoin":             {"Bitcoin", "比特币", "比特幣"},
		"icons.retirement":          {"Retirement", "退休", "退休"},
		"icons.pension":             {"Pension", "养老金", "退休金"},
		"icons.savings":             {"Savings", "储蓄", "儲蓄"},
		"icons.insurance":           {"Insurance", "保险", "保險"},
		"icons.shield":              {"Protection", "保障", "保障"},
		"icons.shieldPlus":          {"Extended protection", "增强保障", "加強保障"},
		"icons.goal":                {"Financial goal", "财务目标", "財務目標"},
		"icons.armchair":            {"Retirement life", "退休生活", "退休生活"},
		"icons.circleCheck":         {"Verified", "已确认", "已確認"},
		"icons.calendar":            {"Calendar", "日历", "日曆"},
		"icons.calendarClock":       {"Schedule", "日程", "日程"},
		"icons.clock":               {"Time", "时间", "時間"},
		"icons.property":            {"Property", "房产", "房產"},
		"icons.liability":           {"Liability", "负债", "負債"},
		"icons.receivable":          {"Receivable", "应收款", "應收款"},
		"icons.home":                {"Home", "家庭", "家庭"},
		"icons.folder":              {"Folder", "文件夹", "資料夾"},
		"icons.document":            {"Document", "文档", "文件"},
		"icons.file":                {"File", "文件", "檔案"},
		"icons.search":              {"Search", "搜索", "搜尋"},
		"icons.settings":            {"Settings", "设置", "設定"},
		"icons.warning":             {"Warning", "警告", "警告"},
		"icons.info":                {"Information", "信息", "資訊"},
		"icons.download":            {"Download", "下载", "下載"},
		"icons.upload":              {"Upload", "上传", "上傳"},
		"icons.visibility":          {"Visibility", "可见性", "可見性"},
		"icons.mail":                {"Mail", "邮件", "郵件"},
		"icons.media":               {"Image", "图片", "圖片"},
		"icons.camera":              {"Camera", "相机", "相機"},
		"icons.computer":            {"Computer", "电脑", "電腦"},
		"icons.grid":                {"Grid", "网格", "網格"},
		"icons.list":                {"List", "列表", "清單"},
		"icons.history":             {"History", "历史", "歷史"},
	}
	for key, values := range translations {
		catalogs[settings.LanguageEnglish][key] = values[0]
		catalogs[settings.LanguageZhCN][key] = values[1]
		catalogs[settings.LanguageZhTW][key] = values[2]
	}
}
func init() {
	catalogs[settings.LanguageEnglish]["dashboard.accountCount"] = "%d accounts"
	catalogs[settings.LanguageZhCN]["dashboard.accountCount"] = "%d 个账户"
	catalogs[settings.LanguageZhTW]["dashboard.accountCount"] = "%d 個帳戶"
}

// Checkbox-based ownership editor: replaces the earlier free-text owner and
// percentage fields, which could not disambiguate members with the same
// name and could not represent an archived member who already owns an
// Account.
func init() {
	catalogs[settings.LanguageEnglish]["accounts.ownershipSharePlaceholder"] = "Optional — split evenly if blank"
	catalogs[settings.LanguageZhCN]["accounts.ownershipSharePlaceholder"] = "可选，留空则自动平均分配"
	catalogs[settings.LanguageZhTW]["accounts.ownershipSharePlaceholder"] = "可選，留空則自動平均分配"
	catalogs[settings.LanguageEnglish]["accounts.ownershipHint"] = "Check one or more owners. Leave every percentage blank to split evenly, or fill in every checked owner's percentage so they total 100%."
	catalogs[settings.LanguageZhCN]["accounts.ownershipHint"] = "勾选一位或多位所有人。所有比例留空将自动平均分配；如需自定义，请为每位勾选的所有人都填写比例，且总和为 100%。"
	catalogs[settings.LanguageZhTW]["accounts.ownershipHint"] = "勾選一位或多位所有人。所有比例留空將自動平均分配；如需自訂，請為每位勾選的所有人都填寫比例，且總和為 100%。"
	catalogs[settings.LanguageEnglish]["settings.numbers.currency"] = "Sample display currency (preview only)"
	catalogs[settings.LanguageZhCN]["settings.numbers.currency"] = "示例展示货币（仅用于下方预览）"
	catalogs[settings.LanguageZhTW]["settings.numbers.currency"] = "示例展示貨幣（僅用於下方預覽）"
}

// Read-only Household summary card required by the v0.1.1 release contract
// (docs/releases/v0.1.1.md); see BUG-4/GAP-1 in
// docs/development/code-review-2026-08-21.md.
func init() {
	catalogs[settings.LanguageEnglish]["settings.household.title"] = "Household"
	catalogs[settings.LanguageZhCN]["settings.household.title"] = "家庭"
	catalogs[settings.LanguageZhTW]["settings.household.title"] = "家庭"
	catalogs[settings.LanguageEnglish]["settings.household.description"] = "Set once during onboarding. Read-only here."
	catalogs[settings.LanguageZhCN]["settings.household.description"] = "在引导流程中设置一次，此处仅供查看。"
	catalogs[settings.LanguageZhTW]["settings.household.description"] = "在引導流程中設定一次，此處僅供查看。"
	catalogs[settings.LanguageEnglish]["settings.household.name"] = "Household name"
	catalogs[settings.LanguageZhCN]["settings.household.name"] = "家庭名称"
	catalogs[settings.LanguageZhTW]["settings.household.name"] = "家庭名稱"
	catalogs[settings.LanguageEnglish]["settings.household.baseCurrency"] = "Base currency"
	catalogs[settings.LanguageZhCN]["settings.household.baseCurrency"] = "基础货币"
	catalogs[settings.LanguageZhTW]["settings.household.baseCurrency"] = "基礎貨幣"
}
