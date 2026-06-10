# アイテムテンプレートのリモートリスト

spawn するアイテム＋impulse タグのテンプレート一覧を、アプリに焼き込まず **リポジトリから配信**
する仕組み。本ドキュメントが正本（2026-06-10 確定・同日スポーン＆パルスで2系統に拡張）。

利用箇所は2系統（機構・スキーマは共通＝`templateStore` の2インスタンス）:

| 系統 | 配信ファイル | 利用箇所 | API |
|---|---|---|---|
| 告知 | `assets/announce-templates.json` | 事前アクション③（§3.16(2)・全ワールド対象） | `GET /api/v1/announce-templates` |
| スポーン＆パルス | `assets/spawn-templates.json` | セッションタブ（フォーカス中ワールドのみ） | `GET /api/v1/spawn-templates` |

## 1. 目的

- 告知/スポーン用アイテムを更新（Resonite 上で保存し直すと resrec URL が変わる）しても、
  **アプリのリリース・各インスタンスでの再設定なしに** 全ユーザー・全 config へ反映する。
- テンプレートの追加も同様に JSON の編集だけで配信する。

## 2. 配信ファイル（正本）

リポジトリ **main ブランチ** の raw URL から取得:

```
https://raw.githubusercontent.com/MarkN2000/MarkNResoniteHeadlessController/main/assets/announce-templates.json
https://raw.githubusercontent.com/MarkN2000/MarkNResoniteHeadlessController/main/assets/spawn-templates.json
```

```jsonc
{
  "version": 1,            // 情報用（互換判定には使わない）
  "templates": [
    {
      "id": "torazo-close",                      // 永続キー（config の announce.templateId / 実行リクエストから参照）
      "label": { "ja": "とらぞセッション閉店アナウンス", "en": "Torazo session closing announce" },
      "url": "resrec:///U-MarkN/R-...",          // spawn するアイテム
      "tag": "MRHC.play"                         // dynamicimpulsestring のタグ
    }
  ]
}
```

### 運用ルール（重要）

- **`id` は改名・削除しない**。既存 config（`templateId`）や利用者の操作の参照が切れるため。
  アイテムの更新は **`url` の書き換え** で行う（これがこの設計の眼目）。
- スキーマ変更は **フィールド追加のみ**（古いアプリは未知キーを無視するため互換）。
- `label` は言語コード→表示名のマップ。**JSON 側への言語追加と UI 側の対応言語追加は互いに独立**
  （UI は 現在言語→en→ja→先頭→id の順でフォールバック表示・`web/src/lib/itemTemplates.ts`）。
- main ブランチ参照のため反映はリリースと無関係に即時（raw の CDN キャッシュ約5分以内）。
- 不正エントリ（id/url/tag いずれか空）は読み手側でスキップされる。有効0件は取得失敗扱い。

## 3. 利用箇所のセマンティクス

### 3a. 告知（事前アクション③・`restart.preActions.announce`）

```jsonc
{ "enabled": true, "templateId": "torazo-close", "itemUrl": "", "impulseTag": "", "message": "" }
```

- **`templateId` 非空＝テンプレ参照**: URL/タグは保存せず、**告知実行の直前に** リストから解決する
  （`orchestrator.announce` → `Server.resolveAnnounce`）。リスト更新が保存済み config にも即反映される。
- **`templateId` 空＝手動入力**: 従来どおり `itemUrl`/`impulseTag` を使う。`Restart.Validate` のタグ必須
  検証は手動時のみ。旧スキーマの config（templateId 無し）は手動入力として無修正で動き続ける
  （マイグレーション・互換コードは作らない方針）。
- 既定値（`config.DefaultRestart`）は `templateId: "torazo-close"`（告知ビルトイン先頭・OFF）。
- 全ワールド対象・spawn→**10秒**→impulse の2パス（v1 実証値。無人・再起動直前の一発勝負のため保守的に）。

### 3b. スポーン＆パルス（セッションタブ・`POST /api/v1/sessions/{idx}/spawn-impulse`）

- body: `{templateId, itemUrl, impulseTag, message}`（templateId 非空=テンプレ参照・空=手動）。
- **フォーカス中ワールドのみ**を対象に spawn → **5秒** 待機 → impulse を**1リクエストで完走**
  （その間レスポンスはブロック＝途中でブラウザを閉じても impulse まで届く）。
  待機が告知の10秒より短いのは、対話操作は失敗しても即再実行できるため（リスク非対称・2026-06-10 裁定）。
- spawn は `active=true / persistent=false` 固定（告知③と同じ・一時アイテム）。`itemUrl` 空は spawn 省略で
  impulse のみ。待機中は execMu を保持しない（spawn/impulse を別 ExecGroup に分離）。
- 設定としては保存しない（その場実行・UI 状態のみ）。
- 未知の `templateId` は 400（対話操作なのでエラーを即返す。告知の「スキップしてログ」とは異なる）。

## 4. 取得・フォールバック（`internal/server/announce_templates.go` の `templateStore`）

常にこの順で、最悪でもビルトイン（＝従来同等）に退化する:

```
メモリキャッシュ(TTL 10分) → リモート取得(timeout 10s・1MB上限)
  → -data/{announce,spawn}-templates.json(最終取得成功分の永続キャッシュ) → 焼き込みビルトイン
```

- 取得成功時にメモリ＋永続キャッシュを更新。`source` は remote / cache / builtin。
- ビルトイン（`builtinAnnounceTemplates` / `builtinSpawnTemplates`）は配信 JSON のスナップショット。
  **告知ビルトインの先頭 id は `config.DefaultRestart()` と同期すること**。
- 取得元はテスト用に `templateStore.url` で差し替え可能。

## 5. 検証 / UI

- `PUT /api/v1/restart-config`: 告知 **有効** かつ `templateId` 非空なら実在を検証（無ければ 400）。
  無効時は検証しない（テンプレ消滅時でも無効化保存を妨げない）。
- 告知実行時に解決できない `templateId`（リストから消えた等の異常系）は **告知をスキップしてログ**
  （誤アイテムの spawn より安全側）。再起動フロー自体は続行。
- UI 共通部品: 言語フォールバック・「手動入力」番兵・選択肢の組み立て（消滅id補完含む）は
  `web/src/lib/itemTemplates.ts`、一覧取得は `web/src/hooks/useItemTemplates.ts`。
  スケジュール（`PreActionsCard`）とセッション（`SpawnImpulseCard`）の両カードが共用する。
- スケジュール側は保存値 `templateId` から選択状態を導出。セッション側は未操作なら先頭テンプレを既定選択。
  テンプレ選択時に解決後 URL は表示しない（シンプル優先・2026-06-10 ユーザー裁定）。

## 6. アイテムを更新/追加する手順（運用者向け）

1. Resonite で新しいアイテムをインベントリ保存し、resrec URL を控える。
2. 対象系統の JSON（`assets/announce-templates.json` または `assets/spawn-templates.json`）の該当エントリの
   `url` を書き換える（追加なら新エントリ。id は新規・恒久）。
3. main へコミット/プッシュ。数分以内に全インスタンスの次回実行・UI 一覧へ反映される。
4. ビルトイン同期: `internal/server/announce_templates.go` の `builtinAnnounceTemplates` /
   `builtinSpawnTemplates` も同じ内容に更新しておく（必須ではないが、オフライン初回起動時の
   フォールバック品質を保つ）。

## 7. テスト

`internal/server/announce_templates_test.go`: フォールバック連鎖（remote/cache/builtin）・TTL・
不正エントリスキップ・有効0件・2系統の独立性・resolveAnnounce（手動/テンプレ/未知id）・PUT 検証・
orchestrator 実行時解決（解決済み URL で spawn / 未解決はスキップ）・spawn-impulse エンドポイント
（手動/テンプレ/impulse のみ/未知id 400/タグ欠落 400・fakehl 統合）。
`internal/config/restart_test.go`: テンプレ参照時はタグ空でも Validate を通る。
