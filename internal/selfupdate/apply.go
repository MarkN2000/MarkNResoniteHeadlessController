// apply.go は更新の適用（DL→検証→入れ替え）の本体。既存バイナリに触るのは
// 全検証が通った後の swapBinary の一瞬だけで、それ以前のどの失敗でも現状維持になる。
package selfupdate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// maxSumsSize は SHA256SUMS の読み取り上限（実体は数百バイト）。
const maxSumsSize = 1 << 20

// lockStaleAfter はこの時間より古いロックファイルを中断の残骸とみなす閾値。
// 正常な Apply は DLClient の Timeout（15分）までに必ず終わるため、それより長く取る。
const lockStaleAfter = time.Hour

// Apply は最新リリースへ自分自身を入れ替え、適用したタグ（例 "v2.1.0"）を返す。
// 成功後も実行中のプロセスは旧版のまま動き続け、次回起動から新版になる。
// 更新不要なら ErrUpToDate、進行中の更新があれば ErrBusy（いずれも sentinel）。
func (u *Updater) Apply(ctx context.Context) (string, error) {
	if !semver.IsValid(u.Version) {
		return "", ErrNotReleaseBuild
	}
	assetFile, err := assetForPlatform(platformKey)
	if err != nil {
		return "", err
	}
	// タグは Apply 自身で解決し直し、比較もここで強制する（Check との間に latest が
	// 動いた場合や、latest が過去版へ付け替えられた場合にダウングレードさせない）。
	latest, err := u.resolveLatestTag(ctx)
	if err != nil {
		return "", err
	}
	if semver.Compare(latest, u.Version) <= 0 {
		return "", ErrUpToDate
	}

	// exe ディレクトリ単位の排他（UI と CLI は別プロセスで in-process の排他が効かない）。
	unlock, err := acquireLock(u.ExePath + ".update.lock")
	if err != nil {
		return "", err
	}
	defer unlock()

	// DL 先は exe と同じディレクトリ（＝同一ボリューム。後段の rename を原子的にするため）。
	// 一時ファイル作成を通信より先に行うことで、書込不可（sudo 展開等）はネットワークを
	// 使う前に os.IsPermission で判定できるエラーとして返る。
	exeDir := filepath.Dir(u.ExePath)
	tmp, err := os.CreateTemp(exeDir, ".mrhc-update-*.partial")
	if err != nil {
		return "", fmt.Errorf("一時ファイルを作成できません（設置ディレクトリに書込権が必要です）: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // 成否にかかわらずアーカイブは残さない

	base := strings.TrimSuffix(u.BaseURL, "/") + "/releases/download/" + latest
	dlErr := u.fetchToWriter(ctx, base+"/"+assetFile, tmp)
	closeErr := tmp.Close()
	if dlErr != nil {
		return "", dlErr
	}
	if closeErr != nil {
		return "", fmt.Errorf("一時ファイルのクローズに失敗: %w", closeErr)
	}

	// SHA256SUMS も同じタグ配下から取得（latest 固定リンクは使わない＝常に同一リリース内で一貫）。
	expected, err := u.fetchExpectedSHA(ctx, base+"/SHA256SUMS", assetFile)
	if err != nil {
		return "", err
	}
	// ハッシュは DL ストリームではなくディスクから読み直して計算する。並行書込・書込経路の
	// 破損があった場合、ストリーム側ハッシュでは検出できずディスク上の壊れた実体を通してしまう。
	actual, err := sha256File(tmpPath)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(actual, expected) {
		return "", fmt.Errorf("SHA-256 が一致しません（期待=%s 実際=%s）。公開直後の場合は数分後に再実行してください", expected, actual)
	}

	newPath := u.ExePath + ".new"
	if err := extractBinary(tmpPath, assetFile, archiveEntryName(assetFile, platformKey), newPath); err != nil {
		os.Remove(newPath)
		return "", err
	}
	if err := verifyBinaryFormat(newPath, platformKey); err != nil {
		os.Remove(newPath)
		return "", err
	}
	if err := swapBinary(u.ExePath, newPath); err != nil {
		os.Remove(newPath)
		return "", err
	}
	return latest, nil
}

// fetchToWriter は url の内容を w へストリームコピーする（サイズ上限つき）。
func (u *Updater) fetchToWriter(ctx context.Context, url string, w io.Writer) error {
	resp, err := u.get(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	n, err := io.Copy(w, io.LimitReader(resp.Body, maxBinarySize+1))
	if err != nil {
		return fmt.Errorf("ダウンロード中の読み込みに失敗: %w", err)
	}
	if n > maxBinarySize {
		return fmt.Errorf("ダウンロードがサイズ上限（%d MiB）を超えています", maxBinarySize>>20)
	}
	return nil
}

// fetchExpectedSHA は SHA256SUMS（"hex␣␣filename" 行の列挙）から file の期待ハッシュを取り出す。
func (u *Updater) fetchExpectedSHA(ctx context.Context, url, file string) (string, error) {
	resp, err := u.get(ctx, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(io.LimitReader(resp.Body, maxSumsSize))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[1] == file {
			return fields[0], nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("SHA256SUMS の読み取りに失敗: %w", err)
	}
	return "", fmt.Errorf("SHA256SUMS に %q の項目がありません", file)
}

func (u *Updater) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MRHC")
	resp, err := u.DLClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ダウンロードに失敗: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("ダウンロードに失敗: HTTP %d (%s)。公開直後の場合は数分後に再実行してください", resp.StatusCode, url)
	}
	return resp, nil
}

// sha256File は path の内容の SHA-256（小文字hex）を返す。
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("検証のための読み取りに失敗: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("検証のための読み取りに失敗: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// acquireLock は lockPath を O_CREATE|O_EXCL で作成して解放関数を返す。既存ロックが
// lockStaleAfter より古ければ中断の残骸として除去して1回だけ取り直す。取れなければ ErrBusy。
func acquireLock(lockPath string) (func(), error) {
	for retry := 0; ; retry++ {
		f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid()) // 調査用（誰が掴んでいるか）
			f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("ロックファイルを作成できません: %w", err)
		}
		fi, statErr := os.Stat(lockPath)
		if retry > 0 || statErr != nil || time.Since(fi.ModTime()) < lockStaleAfter {
			return nil, ErrBusy
		}
		_ = os.Remove(lockPath) // stale ロックを除去して取り直す
	}
}

// swapBinary は検証済みの newPath を exePath へ2段 rename で入れ替える。
//   - Windows では実行中イメージ（exe 本体・退避済みの .old）は削除不可だがリネームは可能、
//     という性質に依存する。.old の削除に失敗する＝何かの実行中イメージなので、一意名へ
//     退避先を変える（再起動せず2回目の更新を実行したケースがこれに当たる）。
//   - rename 2回目はウイルス対策ソフト等の一時オープンで失敗しうるため短いリトライを行い、
//     それでも失敗したら退避した旧バイナリを戻す（exe 不在の状態を残さない）。
func swapBinary(exePath, newPath string) error {
	oldPath := exePath + ".old"
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		oldPath = fmt.Sprintf("%s.old-%d", exePath, time.Now().Unix())
	}
	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("現在のバイナリの退避に失敗: %w", err)
	}
	var renameErr error
	for i := 0; i < 3; i++ {
		if renameErr = os.Rename(newPath, exePath); renameErr == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = os.Rename(oldPath, exePath) // 旧状態の復元を試みる（best-effort）
	return fmt.Errorf("新しいバイナリの配置に失敗: %w", renameErr)
}
