package steam

import (
	"errors"
	"path/filepath"
)

// BuildUpdateParams は資格と install 先から UpdateParams を組む。
// server（steamParams）とウィザード S5 の共有入口（二重実装によるドリフト防止・M5）。
// headlessBinaryName は OS 別の実行ファイル名（platform.HeadlessBinaryName()）を
// 呼び出し側が注入する（steam を OS 非依存に保つ既存の DI 流儀）。
// installDir は解決済み（既定導出・"~" 展開済み）を渡すこと。
// 資格のいずれかが欠ければ ErrSteamNotConfigured。
func BuildUpdateParams(username, password, branchCode, installDir, headlessBinaryName string) (UpdateParams, error) {
	if username == "" || password == "" || branchCode == "" {
		return UpdateParams{}, ErrSteamNotConfigured
	}
	return UpdateParams{
		Username:   username,
		Password:   password,
		BranchCode: branchCode,
		InstallDir: installDir,
		// DL 後に headless 実体が取れたかを検査する相対パス（H2: ブランチコード誤りの
		// public フォールバックを exit 0 でも検出する）。
		VerifyRelPath: filepath.Join("Headless", headlessBinaryName),
	}, nil
}

// ValidatePassword は Steam パスワードの制約（ASCII 限定・最大64文字）を検証する。
// DepotDownloader が ASCII 以外を受け付けないため（password.All(char.IsAscii)・最大64）。
// server の設定 API とウィザード S5a の共有（M4）。
func ValidatePassword(pw string) error {
	if len(pw) > 64 {
		return errors.New("Steam パスワードは64文字以内で入力してください")
	}
	for _, r := range pw {
		if r > 127 {
			return errors.New("Steam パスワードは ASCII 文字のみ使用できます")
		}
	}
	return nil
}
