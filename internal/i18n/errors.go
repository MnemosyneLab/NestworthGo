package i18n

import (
	"errors"
	"fmt"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

// errorMessageKeys maps the stable English message text emitted by the
// domain/application/infrastructure layers to a catalog key. Keeping this
// mapping in the presentation package lets those layers remain independent of
// the active UI language.
var errorMessageKeys = map[string]string{
	"must be a lowercase UUID":                     "error.id.lowercaseUUID",
	"must be an RFC 3339 timestamp":                "error.timestamp.rfc3339",
	"must not be empty":                            "error.validation.notEmpty",
	"must be 120 characters or fewer":              "error.validation.nameLength",
	"must be 2000 characters or fewer":             "error.validation.noteLength",
	"must use YYYY-MM-DD":                          "error.validation.dateFormat",
	"must contain exactly three uppercase letters": "error.validation.currencyFormat",
	"must be a canonical non-negative decimal with up to four fractional digits": "error.validation.moneyFormat",
	"is outside the supported range":                                             "error.validation.amountRange",
	"money currencies must match":                                                "error.validation.moneyCurrencyMatch",
	"is not supported":                                                           "error.validation.unsupported",
	"at least one owner is required":                                             "error.ownership.atLeastOneOwner",
	"owner ID is invalid":                                                        "error.ownership.ownerIDInvalid",
	"each share must be between 1 and 10000 basis points":                        "error.ownership.shareRange",
	"an owner may appear only once":                                              "error.ownership.duplicateOwner",
	"shares must total exactly 10000 basis points":                               "error.ownership.total",
	"at least one member is required":                                            "error.onboarding.memberRequired",
	"percentage must be between 0 and 100 with at most two decimals":             "error.ownership.percentageFormat",
	"percentage must be between 0 and 100":                                       "error.ownership.percentageRange",
	"percentage supports at most two decimals":                                   "error.ownership.percentagePrecision",
	"does not belong to primary category":                                        "error.account.secondaryCategory",
	"is not allowed for this category":                                           "error.account.trackingCategory",
	"holdings accounts are planned for v0.1.2":                                   "error.account.holdingsPlanned",
	"cannot be earlier than openedOn":                                            "error.account.closedBeforeOpened",
	"is required for Balance and Manual Value accounts":                          "error.account.initialValueRequired",
	"account values are not valid for this tracking mode":                        "error.account.valueTrackingMode",
	"currency must match account currency":                                       "error.account.currencyMatch",
	"a Household already exists":                                                 "error.onboarding.householdExists",
	"complete onboarding first":                                                  "error.onboarding.required",
	"member was not found":                                                       "error.notFound.member",
	"institution was not found":                                                  "error.notFound.institution",
	"group was not found":                                                        "error.notFound.group",
	"account was not found":                                                      "error.notFound.account",
	"media asset was not found":                                                  "error.notFound.mediaAsset",
	"household was not found":                                                    "error.notFound.household",
	"unsupported image type":                                                     "error.media.unsupportedType",
	"image is invalid or exceeds the local size limit":                           "error.media.invalidOrTooLarge",
	"v0.1.1 accounts must use the Household base currency":                       "error.account.householdCurrency",
	"institution is not an active reference":                                     "error.reference.institutionInactive",
	"group is not an active reference":                                           "error.reference.groupInactive",
	"account has no current value":                                               "error.account.noCurrentValue",
	"tracking mode is immutable after account creation":                          "error.account.trackingImmutable",
	"account currency is immutable and must match the Household base currency":   "error.account.currencyImmutable",
	"one percentage is required for each owner":                                  "error.ownership.percentageRequired",
	"new owners must be active members":                                          "error.ownership.newOwnerActive",
	"all owners must be active members":                                          "error.ownership.ownerActive",
	"household must retain at least one active member":                           "error.member.lastActive",
	"unsupported media reference":                                                "error.media.unsupportedReference",
	"unsupported archive table":                                                  "error.archive.unsupportedTable",
	"initial value does not match the account":                                   "error.account.initialValueMismatch",
	"institutions reference was not found":                                       "error.notFound.institutionReference",
	"account_groups reference was not found":                                     "error.notFound.groupReference",
	"reference belongs to another household":                                     "error.reference.otherHousehold",
	"reference is archived":                                                      "error.reference.archived",
	"owner belongs to another household":                                         "error.ownership.otherHousehold",
	"timezone cannot be empty":                                                   "error.settings.timezoneEmpty",
	"decimal and grouping separators must differ":                                "error.settings.separatorsMatch",
	"unsupported icon":                                                           "error.icon.unsupported",
	"icon is required":                                                           "error.icon.required",
}

// fieldLabelKeys translates the field prefix added by domain.Error.Error.
// Unknown fields intentionally remain readable in English as a safe fallback.
var fieldLabelKeys = map[string]string{
	"householdId":       "error.field.householdID",
	"memberId":          "error.field.memberID",
	"institutionId":     "nav.institutions",
	"groupId":           "nav.groups",
	"accountId":         "error.field.accountID",
	"accountValueId":    "error.field.accountValueID",
	"mediaAssetId":      "error.field.mediaAssetID",
	"iconKey":           "error.field.iconKey",
	"timestamp":         "error.field.timestamp",
	"name":              "accounts.name",
	"note":              "accounts.note",
	"openedOn":          "accounts.openedOn",
	"closedOn":          "accounts.closedOn",
	"currency":          "error.field.currency",
	"amount":            "accounts.amount",
	"primaryCategory":   "accounts.category",
	"secondaryCategory": "accounts.secondaryCategory",
	"trackingMode":      "accounts.trackingMode",
	"ownership":         "accounts.owner",
	"members":           "onboarding.members",
	"initialAmount":     "accounts.amount",
	"effectiveAt":       "accounts.effectiveDate",
	"defaultCurrency":   "settings.household.baseCurrency",
	"mimeType":          "error.field.mimeType",
	"data":              "error.field.data",
	"initialValue":      "error.field.initialValue",
	"institutions":      "nav.institutions",
	"account_groups":    "nav.groups",
}

type errorTranslation struct {
	english     string
	simplified  string
	traditional string
}

var errorTranslations = map[string]errorTranslation{
	"error.id.lowercaseUUID":              {"must be a lowercase UUID", "必须是小写 UUID", "必須是小寫 UUID"},
	"error.timestamp.rfc3339":             {"must be an RFC 3339 timestamp", "必须是 RFC 3339 时间戳", "必須是 RFC 3339 時間戳"},
	"error.validation.notEmpty":           {"must not be empty", "不能为空", "不能為空"},
	"error.validation.nameLength":         {"must be 120 characters or fewer", "长度不能超过 120 个字符", "長度不能超過 120 個字元"},
	"error.validation.noteLength":         {"must be 2000 characters or fewer", "长度不能超过 2000 个字符", "長度不能超過 2000 個字元"},
	"error.validation.dateFormat":         {"must use YYYY-MM-DD", "必须使用 YYYY-MM-DD", "必須使用 YYYY-MM-DD"},
	"error.validation.currencyFormat":     {"must contain exactly three uppercase letters", "必须恰好包含三个大写字母", "必須恰好包含三個大寫字母"},
	"error.validation.moneyFormat":        {"must be a canonical non-negative decimal with up to four fractional digits", "必须是规范的非负小数，且最多包含四位小数", "必須是規範的非負小數，且最多包含四位小數"},
	"error.validation.amountRange":        {"is outside the supported range", "超出支持的范围", "超出支援的範圍"},
	"error.validation.moneyCurrencyMatch": {"money currencies must match", "金额的货币必须一致", "金額的貨幣必須一致"},
	"error.validation.unsupported":        {"is not supported", "不受支持", "不受支援"},
	"error.ownership.atLeastOneOwner":     {"at least one owner is required", "至少需要一位所有人", "至少需要一位所有人"},
	"error.ownership.ownerIDInvalid":      {"owner ID is invalid", "所有人 ID 无效", "所有人 ID 無效"},
	"error.ownership.shareRange":          {"each share must be between 1 and 10000 basis points", "每份比例必须在 1 到 10000 个基点之间", "每份比例必須在 1 到 10000 個基點之間"},
	"error.ownership.duplicateOwner":      {"an owner may appear only once", "同一位所有人只能出现一次", "同一位所有人只能出現一次"},
	"error.ownership.total":               {"shares must total exactly 10000 basis points", "所有比例总和必须恰好为 10000 个基点", "所有比例總和必須恰好為 10000 個基點"},
	"error.onboarding.memberRequired":     {"at least one member is required", "至少需要一名成员", "至少需要一名成員"},
	"error.ownership.percentageFormat":    {"percentage must be between 0 and 100 with at most two decimals", "比例必须在 0 到 100 之间，且最多两位小数", "比例必須在 0 到 100 之間，且最多兩位小數"},
	"error.ownership.percentageRange":     {"percentage must be between 0 and 100", "比例必须在 0 到 100 之间", "比例必須在 0 到 100 之間"},
	"error.ownership.percentagePrecision": {"percentage supports at most two decimals", "比例最多支持两位小数", "比例最多支援兩位小數"},
	"error.account.secondaryCategory":     {"does not belong to primary category", "不属于主类别", "不屬於主類別"},
	"error.account.trackingCategory":      {"is not allowed for this category", "不允许用于此类别", "不允許用於此類別"},
	"error.account.holdingsPlanned":       {"holdings accounts are planned for v0.1.2", "持仓账户计划在 v0.1.2 提供", "持倉帳戶預計在 v0.1.2 提供"},
	"error.account.closedBeforeOpened":    {"cannot be earlier than openedOn", "不能早于开户日期", "不能早於開戶日期"},
	"error.account.initialValueRequired":  {"is required for Balance and Manual Value accounts", "Balance 和 Manual Value 账户必须填写", "Balance 和 Manual Value 帳戶必須填寫"},
	"error.account.valueTrackingMode":     {"account values are not valid for this tracking mode", "账户值不适用于此跟踪模式", "帳戶值不適用於此追蹤模式"},
	"error.account.currencyMatch":         {"currency must match account currency", "货币必须与账户货币一致", "貨幣必須與帳戶貨幣一致"},
	"error.onboarding.householdExists":    {"a Household already exists", "家庭已经存在", "家庭已經存在"},
	"error.onboarding.required":           {"complete onboarding first", "请先完成引导设置", "請先完成引導設定"},
	"error.notFound.member":               {"member was not found", "未找到成员", "找不到成員"},
	"error.notFound.institution":          {"institution was not found", "未找到机构", "找不到機構"},
	"error.notFound.group":                {"group was not found", "未找到分组", "找不到分組"},
	"error.notFound.account":              {"account was not found", "未找到账户", "找不到帳戶"},
	"error.notFound.mediaAsset":           {"media asset was not found", "未找到媒体资源", "找不到媒體資源"},
	"error.notFound.household":            {"household was not found", "未找到家庭", "找不到家庭"},
	"error.media.unsupportedType":         {"unsupported image type", "不支持的图片类型", "不支援的圖片類型"},
	"error.media.invalidOrTooLarge":       {"image is invalid or exceeds the local size limit", "图片无效或超过本地大小限制", "圖片無效或超過本機大小限制"},
	"error.account.householdCurrency":     {"v0.1.1 accounts must use the Household base currency", "v0.1.1 账户必须使用家庭基础货币", "v0.1.1 帳戶必須使用家庭基礎貨幣"},
	"error.reference.institutionInactive": {"institution is not an active reference", "机构不是有效的活动引用", "機構不是有效的活動參照"},
	"error.reference.groupInactive":       {"group is not an active reference", "分组不是有效的活动引用", "分組不是有效的活動參照"},
	"error.account.noCurrentValue":        {"account has no current value", "账户没有当前值", "帳戶沒有目前值"},
	"error.account.trackingImmutable":     {"tracking mode is immutable after account creation", "账户创建后不能修改跟踪模式", "帳戶建立後不能修改追蹤模式"},
	"error.account.currencyImmutable":     {"account currency is immutable and must match the Household base currency", "账户货币不可修改，且必须与家庭基础货币一致", "帳戶貨幣不可修改，且必須與家庭基礎貨幣一致"},
	"error.ownership.percentageRequired":  {"one percentage is required for each owner", "每位所有人都必须填写比例", "每位所有人都必須填寫比例"},
	"error.ownership.newOwnerActive":      {"new owners must be active members", "新增所有人必须是活动成员", "新增所有人必須是活動成員"},
	"error.ownership.ownerActive":         {"all owners must be active members", "所有人都必须是活动成员", "所有人都必須是活動成員"},
	"error.member.lastActive":             {"household must retain at least one active member", "家庭必须至少保留一名活动成员", "家庭必須至少保留一名活動成員"},
	"error.media.unsupportedReference":    {"unsupported media reference", "不支持的媒体引用", "不支援的媒體參照"},
	"error.archive.unsupportedTable":      {"unsupported archive table", "不支持的归档表", "不支援的封存資料表"},
	"error.account.initialValueMismatch":  {"initial value does not match the account", "初始值与账户不匹配", "初始值與帳戶不相符"},
	"error.notFound.institutionReference": {"institutions reference was not found", "未找到机构引用", "找不到機構參照"},
	"error.notFound.groupReference":       {"account_groups reference was not found", "未找到分组引用", "找不到分組參照"},
	"error.reference.otherHousehold":      {"reference belongs to another household", "引用属于其他家庭", "參照屬於其他家庭"},
	"error.reference.archived":            {"reference is archived", "引用已归档", "參照已封存"},
	"error.ownership.otherHousehold":      {"owner belongs to another household", "所有人属于其他家庭", "所有人屬於其他家庭"},
	"error.settings.timezoneEmpty":        {"timezone cannot be empty", "时区不能为空", "時區不能為空"},
	"error.settings.separatorsMatch":      {"decimal and grouping separators must differ", "小数分隔符和分组分隔符必须不同", "小數分隔符和分組分隔符必須不同"},
	"error.field.householdID":             {"Household ID", "家庭 ID", "家庭 ID"},
	"error.field.memberID":                {"Member ID", "成员 ID", "成員 ID"},
	"error.field.accountID":               {"Account ID", "账户 ID", "帳戶 ID"},
	"error.field.accountValueID":          {"Account value ID", "账户值 ID", "帳戶值 ID"},
	"error.field.mediaAssetID":            {"Media asset ID", "媒体资源 ID", "媒體資源 ID"},
	"error.field.timestamp":               {"Timestamp", "时间戳", "時間戳"},
	"error.field.currency":                {"Currency", "货币", "貨幣"},
	"error.field.mimeType":                {"Image type", "图片类型", "圖片類型"},
	"error.field.data":                    {"Data", "数据", "資料"},
	"error.field.initialValue":            {"Initial value", "初始值", "初始值"},
	"error.field.iconKey":                 {"Icon", "图标", "圖示"},
	"error.icon.unsupported":              {"unsupported icon", "不支持的图标", "不支援的圖示"},
	"error.icon.required":                 {"icon is required", "图标不能为空", "圖示不能為空"},
}

func init() {
	for key, translation := range errorTranslations {
		catalogs[settings.LanguageEnglish][key] = translation.english
		catalogs[settings.LanguageZhCN][key] = translation.simplified
		catalogs[settings.LanguageZhTW][key] = translation.traditional
	}
}

// TranslateError renders an error for the translator's active language. A
// missing table entry deliberately returns the original error text so a new
// error remains visible until its localization is added.
func (t *Translator) TranslateError(err error) string {
	if err == nil {
		return ""
	}
	key, ok := errorMessageKeys[baseMessage(err)]
	if !ok {
		return err.Error()
	}
	message := t.T(key)
	var domainErr *domain.Error
	if errors.As(err, &domainErr) && domainErr != nil && domainErr.Field != "" {
		if labelKey, ok := fieldLabelKeys[domainErr.Field]; ok {
			return fmt.Sprintf("%s: %s", t.T(labelKey), message)
		}
	}
	return message
}

func baseMessage(err error) string {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) && domainErr != nil {
		return domainErr.Message
	}
	return err.Error()
}
