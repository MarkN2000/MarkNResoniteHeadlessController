import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import ja from "./locales/ja.json";
import en from "./locales/en.json";

// 対応言語（UI の言語選択の単一情報源）。
// 言語追加 = ①locales に JSON 追加 ②下の resources に登録 ③この配列に1行。
export const LANGUAGES = [
  { code: "ja", label: "日本語" },
  { code: "en", label: "English" },
] as const;

export type Lang = (typeof LANGUAGES)[number]["code"];

const SUPPORTED: string[] = LANGUAGES.map((l) => l.code);

// 初期言語の決定:
//   1. 手動設定（localStorage）があれば優先＝明示的な上書き
//   2. なければブラウザ言語から自動判定（navigator.languages を順に prefix 一致で探す）
function detectInitialLang(): string {
  const saved = localStorage.getItem("lang");
  if (saved && SUPPORTED.includes(saved)) return saved;
  const langs = navigator.languages?.length ? navigator.languages : [navigator.language || ""];
  for (const l of langs) {
    const lc = l.toLowerCase();
    const hit = SUPPORTED.find((c) => lc.startsWith(c));
    if (hit) return hit;
  }
  return SUPPORTED.includes("en") ? "en" : SUPPORTED[0];
}

i18n.use(initReactI18next).init({
  resources: {
    ja: { translation: ja },
    en: { translation: en },
  },
  lng: detectInitialLang(),
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});

// 手動切替は localStorage に保存し、以降は自動判定より優先する（明示的な上書き）。
export function setLanguage(lng: string) {
  localStorage.setItem("lang", lng);
  void i18n.changeLanguage(lng);
}

export default i18n;
