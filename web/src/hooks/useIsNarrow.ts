import { useMediaQuery } from "@mantine/hooks";

// 狭幅（実機スマホ相当）判定の単一情報源。閾値はここ1箇所（36em=576px・Mantine の xs 相当）。
// getInitialValueInEffect:false で初回描画から matchMedia を読み、PC→モバイルの初回チラつきを防ぐ
// （SSR なしの SPA なので安全）。matchMedia 不在時（万一）は false（=PC扱い）にフォールバック。
export function useIsNarrow(): boolean {
  return useMediaQuery("(max-width: 36em)", false, { getInitialValueInEffect: false }) ?? false;
}
