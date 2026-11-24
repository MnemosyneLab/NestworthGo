package i18n

type Language string

const (
	English            Language = "en"
	SimplifiedChinese  Language = "zh-CN"
	TraditionalChinese Language = "zh-TW"
)

type MessageID uint16

const (
	NoMessage MessageID = iota
	LanguageName
	NavOverview
	NavAccounts
	NavAvailableFunds
	NavPortfolio
	NavTasks
	NavHistory
	NavHistoricalOverview
	NavReturns
	NavAssetChanges
	NavDirectory
	NavDataMaintenance
	NavSettings
	NavComponentPreview
	GroupActivity
	GroupInsights
	GroupManage
	GroupDevelopment
	SampleData
	NetWorth
	Assets
	Liabilities
	InputMethodTest
	InputMethodHint
	PreviewFormHelp
	PreviewInstitutionName
	PreviewInstitutionHint
	PreviewNotes
	PreviewNotesHint
	PreviewNotesHelp
	FieldByteCount
	FieldByteExceeded
	FieldClearSearch
	FieldDismiss
	PreviewSearch
	PreviewSearchHint
	PreviewReadOnly
	PreviewDisabled
	PreviewSubmit
	PreviewNameRequired
	PreviewResults
	PreviewSubmissions
	PreviewSavedName
	PreviewSavedNotes
	PreviewQuery
	PreviewHarbor
	PreviewEvergreen
	PreviewWestlake
	PreviewNoMatches
	PreviewInfo
	PreviewInfoBody
	PickerEmpty
	PickerEmptyOption
	PickerClear
	PickerFilter
	PickerFilterHint
	PickerResultsKeys
	PickerInvalidDate
	PickerInvalidTime
	DatePreviousMonth
	DateNextMonth
	DatePreviousYear
	DateNextYear
	DateToday
	WeekdaySunday
	WeekdayMonday
	WeekdayTuesday
	WeekdayWednesday
	WeekdayThursday
	WeekdayFriday
	WeekdaySaturday
	TimeHours
	TimeMinutes
	TimeAM
	TimePM
	TimeHourDisplay
	TimeTwelveDisplay
	PreviewCash
	PreviewCredit
	PreviewBrokerage
	PreviewHarborBranch
	PreviewAccountType
	PreviewInstitutionChoice
	PreviewEffectiveDate
	PreviewEffectiveTime
	PreviewAccountResult
	PreviewInstitutionResult
	PreviewDateResult
	PreviewTimeResult
	PreviewMondayFirst
	PreviewSundayFirst
	Preview24Hour
	Preview12Hour
	NavInstitutions
	InstitutionsSynthetic
	InstitutionsSearchHint
	InstitutionsRefresh
	InstitutionsRetry
	InstitutionsLoading
	InstitutionsEmpty
	InstitutionsLoadError
	InstitutionsReloadAfterSaveError
	InstitutionsSaved
	InstitutionsEdit
	InstitutionsEditTitle
	InstitutionsEditName
	InstitutionsSave
	InstitutionsSaving
	InstitutionsCancel
	InstitutionsSaveError
	InstitutionsNameLength
	InstitutionBank
	InstitutionBrokerage
	InstitutionInsurer
	InstitutionExchange
	InstitutionEmployer
	InstitutionGovernment
	InstitutionOther
	LightAppearance
	DarkAppearance
	ColorThemeLabel
	IndigoAccent
	LavenderAccent
	MintAccent
	SkyAccent
	PeachAccent
	HistoryCount
	HistoryLoading
	HistoryLoadError
	HistoryRefresh
	HistoryRetry
	HistoryActivity
	HistoryEmpty
	HistoryMore
	HistoryCashIn
	HistoryCashOut
	HistoryCashDividend
	HistoryCashTransfer
	HistoryFXConversion
	HistoryPositionTransfer
	HistoryCostAdjustment
	HistoryBuy
	HistorySell
	HistoryValueUpdate
	HistoryDebtDraw
	HistoryDebtPayment
	HistoryReversal
	messageCount
)

type catalog [messageCount]string

var catalogs = map[Language]*catalog{
	English:            &englishCatalog,
	SimplifiedChinese:  &simplifiedChineseCatalog,
	TraditionalChinese: &traditionalChineseCatalog,
}

func (language Language) Text(id MessageID) string {
	if translated := catalogs[language]; translated != nil && translated[id] != "" {
		return translated[id]
	}
	return catalogs[English][id]
}

func (language Language) Next() Language {
	switch language {
	case English:
		return TraditionalChinese
	case TraditionalChinese:
		return SimplifiedChinese
	default:
		return English
	}
}

func (language Language) UsesChineseDates() bool {
	return language == SimplifiedChinese || language == TraditionalChinese
}
