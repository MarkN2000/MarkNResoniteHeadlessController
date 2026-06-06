package i18n

// catalog は全文言。キーは <領域>.<場面> 形式（例 "wizard.intro", "banner.running"）。
// 文言は docs/design/cli-onboarding.md（ユーザー確定仕様）と一致させること。
// 追加時は ja/en の両方を必ず定義する（欠落・fmt 動詞列の不一致はテストが落とす）。
var catalog = map[string]map[Lang]string{}
