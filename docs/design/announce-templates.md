# 告知テンプレートのリモートリスト

事前アクションの告知（dynamicImpulse・§3.16(2)）で spawn するアイテムのテンプレート一覧を、
アプリに焼き込まず **リポジトリから配信** する仕組み。本ドキュメントが正本（2026-06-10 確定）。

## 1. 目的

- 告知アイテムを更新（Resonite 上で保存し直すと resrec URL が変わる）しても、
  **アプリのリリース・各インスタンスでの再設定なしに** 全ユーザー・全 config へ反映する。
- テンプレートの追加も同様に JSON の編集だけで配信する。

## 2. 配信ファイル（正本）

`assets/announce-templates.json`（このリポジトリ・**main ブランチ**）。取得元 URL:

```
https://raw.githubusercontent.com/MarkN2000/MarkNResoniteHeadlessController/main/assets/announce-templates.json
```

```jsonc
{
  "version": 1,            // 情報用（互換判定には使わない）
  "templates": [
    {
      "id": "torazo-close",                      // 永続キー（config の announce.templateId から参照）
      "label": { "ja": "とらぞセッション閉店アナウンス", "en": "Torazo session closing announce" },
      "url": "resrec:///U-MarkN/R-...",          // spawn するアイテム
      "tag": "MRHC.play"                         // dynamicimpulsestring のタグ
    }
  ]
}
```

### 運用ルール（重要）

- **`id` は改名・削除しない**。既存 config（`templateId`）の参照が切れるため。
  アイテムの更新は **`url` の書き換え** で行う（これがこの設計の眼目）。
- スキーマ変更は **フィールド追加のみ**（古いアプリは未知キーを無視するため互換）。
- `label` は言語コード→表示名のマップ。**JSON 側への言語追加と UI 側の対応言語追加は互いに独立**
  （UI は 現在言語→en→ja→先頭→id の順でフォールバック表示・`scheduleModel.templateLabel`）。
- main ブランチ参照のため反映はリリースと無関係に即時（raw の CDN キャッシュ約5分以内）。
- 不正エントリ（id/url/tag いずれか空）は読み手側でスキップされる。有効0件は取得失敗扱い。

## 3. config スキーマ（`restart.preActions.announce`）

```jsonc
{ "enabled": true, "templateId": "torazo-close", "itemUrl": "", "impulseTag": "", "message": "" }
```

- **`templateId` 非空＝テンプレ参照**: URL/タグは保存せず、**告知実行の直前に** リストから解決する
  （`orchestrator.announce` → `Server.resolveAnnounce`）。リスト更新が保存済み config にも即反映される。
- **`templateId` 空＝手動入力**: 従来どおり `itemUrl`/`impulseTag` を使う。`Restart.Validate` のタグ必須
  検証は手動時のみ。旧スキーマの config（templateId 無し）は手動入力として無修正で動き続ける
  （マイグレーション・互換コードは作らない方針）。
- 既定値（`config.DefaultRestart`）は `templateId: "torazo-close"`（ビルトイン先頭・OFF）。

## 4. 取得・フォールバック（`internal/server/announce_templates.go`）

常にこの順で、最悪でもビルトイン（＝従来同等）に退化する:

```
メモリキャッシュ(TTL 10分) → リモート取得(timeout 10s・1MB上限)
  → -data/announce-templates.json(最終取得成功分の永続キャッシュ) → 焼き込みビルトイン
```

- 取得成功時にメモリ＋永続キャッシュを更新。`source` は remote / cache / builtin。
- ビルトイン（`builtinAnnounceTemplates`）は配信 JSON のスナップショット。
  **先頭 id は `config.DefaultRestart()` と同期すること**。
- 取得元はテスト用に `Server.tplURL` で差し替え可能。

## 5. API / 検証 / UI

- `GET /api/v1/announce-templates` → `{ templates: [...], source }`（requireAuth・常に 200）。
- `PUT /api/v1/restart-config`: 告知 **有効** かつ `templateId` 非空なら実在を検証（無ければ 400）。
  無効時は検証しない（テンプレ消滅時でも無効化保存を妨げない）。
- 実行時に解決できない `templateId`（リストから消えた等の異常系）は **告知をスキップしてログ**
  （誤アイテムの spawn より安全側）。再起動フロー自体は続行。
- UI（`PreActionsCard`）: マウント時に一覧を取得し `Select` を構成。選択状態は `templateId` から導出。
  保存済み id が一覧に無い間（取得前/消滅）は id をそのままラベル表示して選択を保つ。
  テンプレ選択時に解決後 URL は表示しない（シンプル優先・2026-06-10 ユーザー裁定）。

## 6. アイテムを更新/追加する手順（運用者向け）

1. Resonite で新しいアイテムをインベントリ保存し、resrec URL を控える。
2. `assets/announce-templates.json` の該当エントリの `url` を書き換える（追加なら新エントリ。id は新規・恒久）。
3. main へコミット/プッシュ。数分以内に全インスタンスの次回告知・UI 一覧へ反映される。
4. ビルトイン同期: `internal/server/announce_templates.go` の `builtinAnnounceTemplates` も同じ内容に
   更新しておく（必須ではないが、オフライン初回起動時のフォールバック品質を保つ）。

## 7. テスト

`internal/server/announce_templates_test.go`: フォールバック連鎖（remote/cache/builtin）・TTL・
不正エントリスキップ・有効0件・resolveAnnounce（手動/テンプレ/未知id）・PUT 検証・
orchestrator 実行時解決（解決済み URL で spawn / 未解決はスキップ）。
`internal/config/restart_test.go`: テンプレ参照時はタグ空でも Validate を通る。
