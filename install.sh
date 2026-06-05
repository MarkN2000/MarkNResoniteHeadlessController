#!/bin/sh
# MRHC インストーラ — 最新リリースの tar.gz を取得し、カレントディレクトリに
# mrhc-linux-<arch>/ として展開する（Linux 専用）。
#
# 使い方（任意のフォルダで）:
#   curl -fsSL https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest/download/install.sh | sh
#
# - 依存は POSIX sh + curl + tar + uname + mktemp のみ（distro 非依存・sudo 不要）。
# - 外部依存（freetype2 / ARM の .NET 10）の検出・導入案内は MRHC 本体が行う。
# - 同じ場所で再実行するとバイナリだけが上書き＝更新になる
#   （config 等はアーカイブに含まれないため保持される。MRHC は停止してから実行）。
# - MRHC_DOWNLOAD_BASE でダウンロード元ベース URL を差し替え可能
#   （特定タグの固定: .../releases/download/<tag> ・ローカルテスト用）。
#
# curl|sh のパイプ実行は通信切断時に途中までのスクリプトが実行されうるため、
# 全体を main() に包み最終行で呼ぶ（最終行が届かない限り何も実行されない）。

set -eu

main() {
    base="${MRHC_DOWNLOAD_BASE:-https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest/download}"
    base="${base%/}" # 末尾スラッシュを除去（URL が // にならないように）

    for cmd in curl tar uname mktemp; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            echo "エラー: $cmd が見つかりません。導入してから再実行してください。" >&2
            exit 1
        fi
    done

    os="$(uname -s)"
    if [ "$os" != "Linux" ]; then
        echo "エラー: このスクリプトは Linux 専用です（検出: $os）。" >&2
        echo "Windows は zip 版を利用してください: https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest" >&2
        exit 1
    fi

    # armv8l/armv7l（32bit ユーザーランド）や i686 は arm64/amd64 バイナリを実行できない
    # ため対応表に含めず明示エラーにする。
    machine="$(uname -m)"
    case "$machine" in
    x86_64 | amd64) arch="amd64" ;;
    aarch64 | arm64) arch="arm64" ;;
    *)
        echo "エラー: 未対応のアーキテクチャです: $machine（対応: x86_64 / aarch64）" >&2
        exit 1
        ;;
    esac

    name="mrhc-linux-${arch}"
    url="${base}/${name}.tar.gz"

    tmp="$(mktemp)"
    trap 'rm -f "$tmp"' EXIT

    echo "ダウンロード中: $url"
    # curl フラグは位置パラメータで組み立てる（POSIX sh に配列が無いため）。-#=進捗バー。
    # 既定（GitHub）は https 以外を拒否・MRHC_DOWNLOAD_BASE 差し替え時はローカル HTTP 等も許可（テスト用）。
    set -- -fL -# -o "$tmp" "$url"
    if [ -z "${MRHC_DOWNLOAD_BASE:-}" ]; then
        set -- --proto '=https' "$@"
    fi
    if ! curl "$@"; then
        echo "エラー: ダウンロードに失敗しました: $url" >&2
        echo "（リリースが未公開の可能性があります。確認: https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases ）" >&2
        exit 1
    fi

    # tar が実行権を保持するため chmod +x は不要
    if ! tar -xzf "$tmp" -C .; then
        echo "エラー: 展開に失敗しました（ダウンロードが破損している可能性があります。再実行してください）。" >&2
        exit 1
    fi

    echo ""
    echo "展開しました: ./${name}/"
    echo "次の手順:"
    echo "  cd ${name} && ./mrhc"
    echo "（初回はセットアップウィザードが起動します）"
}

# 引数なしで呼ぶ（"$@" は bash 4.3 以前が sh の環境で set -u + 引数ゼロだと
# unbound variable エラーになる既知バグがあり、引数は使わないため不要）
main
