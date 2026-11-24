package i18n

import "strings"

type DirectoryIconCopy struct {
	ID     string
	labels [4]string
}

func (c DirectoryIconCopy) Label(language Language) string {
	switch language {
	case SimplifiedChinese:
		return c.labels[1]
	case TraditionalChinese:
		return c.labels[2]
	case Japanese:
		return c.labels[3]
	default:
		return c.labels[0]
	}
}

func (c DirectoryIconCopy) SearchText() string {
	return c.ID + " " + strings.Join(c.labels[:], " ")
}

func DirectoryIcons() []DirectoryIconCopy { return directoryIcons[:] }

// The order, keys and labels follow the Wails icon catalog. A contract test
// checks them against the source registry and the four shared translations.
var directoryIcons = [...]DirectoryIconCopy{
	{"account", [4]string{"Account", "账户", "帳戶", "口座"}},
	{"wallet", [4]string{"Wallet", "钱包", "錢包", "財布"}},
	{"wallet-cards", [4]string{"Wallet cards", "钱包卡片", "錢包卡片", "ウォレットカード"}},
	{"cash", [4]string{"Cash", "现金", "現金", "現金"}},
	{"coins", [4]string{"Coins", "硬币", "硬幣", "コイン"}},
	{"money", [4]string{"Money", "金钱", "金錢", "お金"}},
	{"card", [4]string{"Card", "卡片", "卡片", "カード"}},
	{"receipt", [4]string{"Receipt", "收据", "收據", "領収書"}},
	{"banknote-up", [4]string{"Money in", "收入", "收入", "入金"}},
	{"term-deposit", [4]string{"Term deposit", "定期存款", "定期存款", "定期預金"}},
	{"bank", [4]string{"Bank", "银行", "銀行", "銀行"}},
	{"bank-branch", [4]string{"Bank branch", "银行网点", "銀行網點", "銀行支店"}},
	{"building", [4]string{"Building", "建筑", "建築", "建物"}},
	{"vault", [4]string{"Vault", "金库", "金庫", "保管庫"}},
	{"storage", [4]string{"Database", "数据库", "資料庫", "データベース"}},
	{"brokerage", [4]string{"Brokerage", "券商", "券商", "証券会社"}},
	{"brokerage-cash", [4]string{"Broker cash", "券商现金", "券商現金", "証券口座の現金"}},
	{"investment", [4]string{"Investment", "投资", "投資", "投資"}},
	{"stock", [4]string{"Stocks", "股票", "股票", "株式"}},
	{"market", [4]string{"Market", "市场", "市場", "市場"}},
	{"chart", [4]string{"Financial chart", "财务图表", "財務圖表", "財務チャート"}},
	{"trending-up", [4]string{"Growth", "增长趋势", "增長趨勢", "成長"}},
	{"percent", [4]string{"Percent", "百分比", "百分比", "パーセント"}},
	{"badge-percent", [4]string{"Rate", "比例", "比例", "レート"}},
	{"circle-percent", [4]string{"Percentage", "百分率", "百分率", "割合"}},
	{"fund", [4]string{"Fund / ETF", "基金 / ETF", "基金 / ETF", "投資信託・ETF"}},
	{"bond", [4]string{"Bond", "债券", "債券", "債券"}},
	{"car", [4]string{"Vehicle", "车辆", "車輛", "車両"}},
	{"gem", [4]string{"Collectible", "收藏品", "收藏品", "収集品"}},
	{"property", [4]string{"Property", "房产", "房產", "財産"}},
	{"home", [4]string{"Home", "家庭", "家庭", "ホーム"}},
	{"land", [4]string{"Land", "土地", "土地", "土地"}},
	{"commercial-property", [4]string{"Commercial property", "商业地产", "商業地產", "商業用不動産"}},
	{"collectible", [4]string{"Collectibles", "收藏品", "收藏品", "収集品"}},
	{"credit-card", [4]string{"Credit card", "信用卡", "信用卡", "クレジットカード"}},
	{"banknote-down", [4]string{"Money out", "支出", "支出", "出金"}},
	{"liability", [4]string{"Liability", "负债", "負債", "負債"}},
	{"receivable", [4]string{"Receivable", "应收款", "應收款", "未収金"}},
	{"lending", [4]string{"Lending", "借贷", "借貸", "貸付"}},
	{"retirement", [4]string{"Retirement", "退休", "退休", "退職"}},
	{"pension", [4]string{"Pension", "养老金", "退休金", "年金"}},
	{"savings", [4]string{"Savings", "储蓄", "儲蓄", "貯蓄"}},
	{"insurance", [4]string{"Insurance", "保险", "保險", "保険"}},
	{"shield", [4]string{"Protection", "保障", "保障", "保護"}},
	{"shield-plus", [4]string{"Extended protection", "增强保障", "加強保障", "拡張保護"}},
	{"goal", [4]string{"Financial goal", "财务目标", "財務目標", "資産形成の目標"}},
	{"armchair", [4]string{"Retirement life", "退休生活", "退休生活", "退職後の生活"}},
	{"circle-check", [4]string{"Verified", "已确认", "已確認", "確認済み"}},
	{"bitcoin", [4]string{"Bitcoin", "比特币", "比特幣", "ビットコイン"}},
	{"digital-asset", [4]string{"Digital assets", "数字资产", "數位資產", "デジタル資産"}},
	{"crypto-logo:btc", [4]string{"Bitcoin (BTC logo)", "比特币（BTC 标志）", "比特幣（BTC 標誌）", "ビットコイン（BTCロゴ）"}},
	{"crypto-logo:eth", [4]string{"Ethereum (ETH logo)", "以太坊（ETH 标志）", "以太坊（ETH 標誌）", "イーサリアム（ETHロゴ）"}},
	{"crypto-logo:sol", [4]string{"Solana (SOL logo)", "Solana（SOL 标志）", "Solana（SOL 標誌）", "ソラナ（SOLロゴ）"}},
	{"crypto-logo:usdc", [4]string{"USD Coin (USDC logo)", "USD Coin（USDC 标志）", "USD Coin（USDC 標誌）", "USDコイン（USDCロゴ）"}},
	{"currency", [4]string{"Currency", "货币", "貨幣", "通貨"}},
	{"dollar", [4]string{"US dollar", "美元", "美元", "米ドル"}},
	{"badge-dollar", [4]string{"Dollar", "美元标记", "美元標記", "ドル"}},
	{"circle-dollar", [4]string{"Dollar coin", "美元硬币", "美元硬幣", "ドル硬貨"}},
	{"euro", [4]string{"Euro", "欧元", "歐元", "ユーロ"}},
	{"badge-euro", [4]string{"Euro badge", "欧元标记", "歐元標記", "ユーロバッジ"}},
	{"yen", [4]string{"Japanese yen", "日元", "日圓", "日本円"}},
	{"badge-yen", [4]string{"Yen badge", "日元标记", "日圓標記", "円バッジ"}},
	{"badge-pound", [4]string{"Pound badge", "英镑标记", "英鎊標記", "ポンドバッジ"}},
	{"badge-rupee", [4]string{"Indian rupee", "印度卢比", "印度盧比", "インドルピー"}},
	{"badge-ruble", [4]string{"Russian ruble", "俄罗斯卢布", "俄羅斯盧布", "ロシアルーブル"}},
	{"badge-franc", [4]string{"Swiss franc", "瑞士法郎", "瑞士法郎", "スイスフラン"}},
	{"calendar", [4]string{"Calendar", "日历", "日曆", "カレンダー"}},
	{"calendar-clock", [4]string{"Schedule", "日程", "日程", "スケジュール"}},
	{"user", [4]string{"Member", "成员", "成員", "メンバー"}},
	{"landmark", [4]string{"Government building", "政府建筑", "政府建築", "庁舎"}},
	{"pie-chart", [4]string{"Portfolio allocation", "资产配置", "資產配置", "ポートフォリオの配分"}},
	{"folder", [4]string{"Folder", "文件夹", "資料夾", "フォルダー"}},
	{"document", [4]string{"Document", "文档", "文件", "書類"}},
	{"file", [4]string{"File", "文件", "檔案", "ファイル"}},
	{"search", [4]string{"Search icons", "搜索图标", "搜尋圖示", "アイコンを検索"}},
	{"settings", [4]string{"Settings", "设置", "設定", "設定"}},
	{"warning", [4]string{"Warning", "警告", "警告", "警告"}},
	{"info", [4]string{"Information", "信息", "資訊", "参考情報"}},
	{"download", [4]string{"Download", "下载", "下載", "ダウンロード"}},
	{"upload", [4]string{"Upload", "上传", "上傳", "アップロード"}},
	{"visibility", [4]string{"Visibility", "可见性", "可見性", "表示"}},
	{"mail", [4]string{"Mail", "邮件", "郵件", "メール"}},
	{"camera", [4]string{"Camera", "相机", "相機", "カメラ"}},
	{"computer", [4]string{"Computer", "电脑", "電腦", "コンピューター"}},
	{"grid", [4]string{"Grid", "网格", "網格", "グリッド"}},
	{"list", [4]string{"List", "列表", "清單", "リスト"}},
	{"history", [4]string{"History", "历史", "歷史", "履歴"}},
}
