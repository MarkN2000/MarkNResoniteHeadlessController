# アイテムテンプレートのリモートリスト

spawn するアイテム＋impulse タグのテンプレート一覧を、アプリに焼き込まず **リポジトリから配信**
する仕組み。本ドキュメントが正本。配信・キャッシュ・ビルトインはいずれも**単一一覧**であり、
各利用箇所は entry の `actions` で選択肢を絞り込む。

| action | 利用箇所 | tag |
|---|---|---|
| `spawn` | セッションタブの単体スポーン | 任意 |
| `spawnImpulse` | セッションタブのスポーン＆パルス | 必須 |
| `announce` | スケジュールタブの再起動前告知 | 必須 |

## 1. 目的

- 告知/スポーン用アイテムを更新（Resonite 上で保存し直すと resrec URL が変わる）しても、
  **アプリのリリース・各インスタンスでの再設定なしに** 全ユーザー・全 config へ反映する。
- テンプレートの追加も同様に JSON の編集だけで配信する。

## 2. 配信ファイル（正本）

リポジトリ **main ブランチ** の raw URL から取得:

```
https://raw.githubusercontent.com/MarkN2000/MarkNResoniteHeadlessController/main/assets/item-templates.json
```

```jsonc
{
  "version": 1,            // 情報用（互換判定には使わない）
  "templates": [
    {
      "id": "tts-voice-loop",                    // 永続キー（config の announce.templateId / 実行リクエストから参照）
      "label": { "ja": "テキスト読み上げボイス指定ループ", "en": "Voice-specified TTS loop" },
      "url": "resrec:///U-MarkN/R-...",          // spawn するアイテム
      "actions": ["spawnImpulse", "announce"],   // 必須。利用できる操作
      "tag": "MRHC.play",                        // spawnImpulse / announce があれば必須
      "input": { "kind": "ttsVoice" }          // 任意。TTS 音声入力を受けるテンプレートのみ
    }
  ]
}
```

現行 entry と `actions` は次のとおり。

| id | actions |
|---|---|
| `torazo-close` | `announce` |
| `tts-loop` | `spawn`, `spawnImpulse`, `announce` |
| `tts-single` | `spawnImpulse`（URL未設定のため現在は無効） |
| `tts-voice-single` | `spawnImpulse`（URL未設定のため現在は無効） |
| `tts-voice-loop` | `spawnImpulse`, `announce` |

### 運用ルール（重要）

- **`id` は改名・削除しない**。既存 config（`templateId`）や利用者の操作の参照が切れるため。
  アイテムの更新は **`url` の書き換え** で行う（これがこの設計の眼目）。
- `label` は言語コード→表示名のマップ。**JSON 側への言語追加と UI 側の対応言語追加は互いに独立**
  （UI は 現在言語→en→ja→先頭→id の順でフォールバック表示・`web/src/lib/itemTemplates.ts`）。
- main ブランチ参照のため反映はリリースと無関係に即時（raw の CDN キャッシュ約5分以内）。
- `actions` は必須の文字列配列で、許可値は `spawn` / `spawnImpulse` / `announce` のみ。空配列・未知値を含む entry は無効。
- `id` / `url` は必須。`actions` に `spawnImpulse` または `announce` がある entry は `tag` も必須である。
  `spawn` のみの entry では `tag` は任意。いずれかを満たさない entry は読み手側でスキップされ、有効0件は取得失敗扱い。
- `input` は任意のオブジェクトで、未指定のテンプレートは従来どおりの**テキストのみ**入力として扱う。現在使用する
  `input.kind` は `"ttsVoice"` のみである。`ttsVoice` でも `url` は必須であり、空 URL は既存どおり不正エントリとして
  除外する。

### TTS 音声入力（`input.kind: "ttsVoice"`）

- `ttsVoice` テンプレートでは、UI は従来のテキスト入力に加えて `speakerId` を必須入力とし、backend が取得した話者の
  `styles` から選択する。話者一覧はクライアントが外部サービスへ直接接続せず、`GET /api/v1/tts-speakers` を介して取得する。
  backend は固定の `https://tts.markn2000.com/api/v1/speakers` を取得元とし、各 style を
  `{id, speakerName, styleName}` に平坦化して返す。
- backend はテキストと選択された style ID を URL クエリとしてエンコードし、
  `https://tts.markn2000.com/api/v1/tts?text=<URL encoded>&speaker=<style id>` という**URL全体**を
  `dynamicimpulsestring` の値にする。従来テンプレートでは、従来どおり入力テキストをそのまま値にする。
- `ttsVoice` では空白だけではないテキストと正の `speakerId` が必須。通常テンプレートと手動入力では
  `speakerId` を送らない（スケジュール設定では `0`）。条件に合わないリクエスト・設定保存は 400 とする。
- `input.kind: "ttsVoice"` は `spawnImpulse` / `announce` 用であり、`spawn` 専用 entry には指定できない。現行 TTS entry は
  `tts-voice-single`（`spawnImpulse`）と `tts-voice-loop`（`spawnImpulse` / `announce`）である。これによりセッションは
  single / loop を選べ、スケジュールでは loop のみを選べる。`input.kind` 自体にモードは追加しない。
- `tts-single` と `tts-voice-single` はユーザーが後で URL を入力する予定だが、テンプレートの `url` は必須である。
  空 URL の entry は妥当性検証により無効として除外され、URL を設定するまではセッションの選択肢に表示されない。

## 3. 利用箇所のセマンティクス

### 3a. 告知（事前アクション③・`restart.preActions.announce`）

```jsonc
{ "enabled": true, "templateId": "tts-voice-loop", "itemUrl": "", "impulseTag": "", "message": "再起動します", "speakerId": 888753762 }
```

- **`templateId` 非空＝`announce` action のテンプレ参照**: URL/タグは保存せず、**告知実行の直前に** 単一リストから解決する
  （`orchestrator.announce` → `Server.resolveAnnounce`）。リスト更新が保存済み config にも即反映される。
- **`templateId` 空＝手動入力**: 従来どおり `itemUrl`/`impulseTag` を使う。`Restart.Validate` のタグ必須
  検証は手動時のみ。旧スキーマの config（templateId 無し）は手動入力として無修正で動き続ける
  （マイグレーション・互換コードは作らない方針）。
- 既定値（`config.DefaultRestart`）は `templateId: "torazo-close"`（告知ビルトイン先頭・OFF）。
- 全ワールド対象。各ワールドで spawn の応答に `Spawned item from URL: <url>` が含まれることを確認し、
  全ワールドのspawn処理後に**500ms待機**し、完了確認できたワールドだけへ impulse を送る。
  spawnコマンドの失敗・完了未確認ではそのワールドのimpulseを省略して巡回を続ける。待機後もindexとセッション名が
  一致するワールドだけを成功ワールドとして扱う。focus自体の失敗時は
  `WorldsService.ForEach` の契約どおり巡回を中断する。500ms待機中は execMu を保持しない。
  spawn コマンドの待機上限は60秒（成功・失敗時はプロンプト復帰で即終了）。
- テンプレートが `ttsVoice` のときは、`message` をテキスト、`speakerId` を話者 style ID として保存する。スケジュールで
  選択できる TTS テンプレートは loop 用のみである。既存のテキストのみテンプレートでは `speakerId` は使わず、従来の
  保存形式・実行値を維持する。

### 3b. スポーン＆パルス（セッションタブ・`POST /api/v1/sessions/{idx}/spawn-impulse`）

- body: `{templateId, itemUrl, impulseTag, message, speakerId?}`（templateId 非空=`spawnImpulse` action のテンプレ参照・空=手動）。
- **フォーカス中ワールドのみ**を対象に、spawn の応答に `Spawned item from URL: <url>` が含まれることを確認し、
  **完了確認から500ms後**に impulse を**1リクエストで完走**する。完了確認できなければエラーを返して impulse は送らない。
  spawn コマンドの待機上限は60秒（成功・失敗時はプロンプト復帰で即終了）。
- spawn は `active=true / persistent=false` 固定（告知③と同じ・一時アイテム）。`itemUrl` 空は spawn 省略で
  impulse のみ。500ms待機中は execMu を保持しない（spawn/impulse を別 ExecGroup に分離）。
- 設定としては保存しない（その場実行・UI 状態のみ）。
- 未知の `templateId` は 400（対話操作なのでエラーを即返す。告知の「スキップしてログ」とは異なる）。
- `ttsVoice` テンプレートでは `message` をテキスト、`speakerId` を話者 style ID として送る。セッションでは single / loop
  の両テンプレートを利用できる。既存のテキストのみテンプレートの body と動作は変えない。

### 3c. アイテムスポーン単体（セッションタブ・既存 `POST /api/v1/sessions/{idx}/spawn`）

- 単一リストから `spawn` action を持つ entry だけをテンプレ選択 UI に表示する。
- 選択中のテンプレの `url` を**クライアント側で**そのまま既存 spawn API に渡すだけ（実行系 backend 無変更・
  `tag`/`message` は未使用）。保存される設定ではなくその場実行のため、告知のような実行時サーバー解決は行わない。
- `active`/`persistent` のチェックボックスはテンプレ選択と独立に効く（単体スポーンの存在意義）。

## 4. 取得・フォールバック（`internal/server/item_templates.go` の単一 `templateStore`）

常にこの順で、最悪でもビルトイン（＝従来同等）に退化する:

```
メモリキャッシュ(TTL 10分) → リモート取得(timeout 10s・1MB上限)
  → -data/item-templates.json(最終取得成功分の永続キャッシュ) → 焼き込みビルトイン
```

- 取得成功時にメモリ＋永続キャッシュを更新。`source` は remote / cache / builtin。
- ビルトイン（`builtinItemTemplates`）は配信 JSON のスナップショット。**既定の告知 templateId は
  `config.DefaultRestart()` と同期すること**。
- 取得元はテスト用に `templateStore.url` で差し替え可能。

## 5. 検証 / UI

- `PUT /api/v1/restart-config`: 告知 **有効** かつ `templateId` 非空なら、実在し `announce` action を持つことを検証（無ければ 400）。
  無効時は検証しない（テンプレ消滅時でも無効化保存を妨げない）。
- 告知実行時に解決できない `templateId`（リストから消えた等の異常系）は **告知をスキップしてログ**
  （誤アイテムの spawn より安全側）。再起動フロー自体は続行。
- UI 共通部品: 言語フォールバック・「手動入力」番兵・`actions` による選択肢の絞り込み（消滅id補完含む）は
  `web/src/lib/itemTemplates.ts`、一覧取得は `web/src/hooks/useItemTemplates.ts`（`GET /api/v1/item-templates`）。
  スケジュール（`PreActionsCard`）とセッション（`SpawnImpulseCard`）の両カードが共用する。
- スケジュール側は保存値 `templateId` から選択状態を導出。セッション側は未操作なら先頭テンプレを既定選択。
  テンプレ選択時に解決後 URL は表示しない（シンプル優先・2026-06-10 ユーザー裁定）。
- `ttsVoice` 選択時だけテキストと話者 style の選択欄を表示する。話者一覧は `GET /api/v1/tts-speakers` を通じて取得し、
  話者の `styles` を選択肢にする。既存のテキストのみテンプレートの入力UIは維持する。

## 6. アイテムを更新/追加する手順（運用者向け）

1. Resonite で新しいアイテムをインベントリ保存し、resrec URL を控える。
2. `assets/item-templates.json` の該当 entry の `url` を書き換える（追加なら新 entry。id は新規・恒久）。
   利用箇所は `actions` で指定する。
3. main へコミット/プッシュ。数分以内に全インスタンスの次回実行・UI 一覧へ反映される。
4. ビルトイン同期: `internal/server/item_templates.go` の `builtinItemTemplates` も同じ内容に更新しておく（必須ではないが、
   オフライン初回起動時のフォールバック品質を保つ）。

## 7. テスト

`internal/server/item_templates_test.go`: フォールバック連鎖（remote/cache/builtin）・TTL・
不正エントリスキップ・有効0件・`actions` による利用箇所の絞り込み・tag の条件付き必須・
resolveAnnounce（手動/テンプレ/未知id）・PUT 検証・
orchestrator 実行時解決（解決済み URL で spawn / 未解決はスキップ）・spawn-impulse エンドポイント
（手動/テンプレ/impulse のみ/未知id 400/タグ欠落 400・fakehl 統合）。
`internal/config/restart_test.go`: テンプレ参照時はタグ空でも Validate を通る。
TTS 音声入力追加時は、`ttsVoice` のテンプレ検出、`/speakers` の backend 経由取得、style 選択、TTS URL のクエリ
エンコード、既存テキストのみテンプレートの非回帰を検証する。
