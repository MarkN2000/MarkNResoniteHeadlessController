// アイテムテンプレート（リモートリスト）の表示ヘルパ（純関数）。
// 告知（スケジュールタブ）とスポーン＆パルス（セッションタブ）の両カードで共用する。
// 正本: docs/design/announce-templates.md
import type { ItemTemplate } from "../api";

// 「手動入力」を表す番兵（テンプレ id には使わない "#" を含むため実テンプレ id と衝突しない）。
export const MANUAL_TEMPLATE = "#manual";

// テンプレ表示名の言語フォールバック: 現在言語 → en → ja → 先頭のラベル → id。
// リモートJSON側の言語追加とUI側の対応言語追加が互いに独立でも壊れないようにする。
export function templateLabel(tpl: ItemTemplate, locale: string): string {
  return tpl.label?.[locale] ?? tpl.label?.en ?? tpl.label?.ja ?? Object.values(tpl.label ?? {})[0] ?? tpl.id;
}

// Select 用の選択肢を組み立てる。currentId（空=該当なし）が一覧に無い間
// （取得前 / リストから消えた異常系）も選択表示が消えないよう id をそのまま
// ラベルにした項目を補い、末尾に「手動入力」を置く。
export function buildTemplateSelectData(
  templates: ItemTemplate[],
  currentId: string,
  locale: string,
  manualLabel: string,
): { value: string; label: string }[] {
  return [
    ...templates.map((tpl) => ({ value: tpl.id, label: templateLabel(tpl, locale) })),
    ...(currentId === "" || templates.some((tpl) => tpl.id === currentId)
      ? []
      : [{ value: currentId, label: currentId }]),
    { value: MANUAL_TEMPLATE, label: manualLabel },
  ];
}
