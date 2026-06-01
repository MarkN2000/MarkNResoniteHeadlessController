import { createTheme, type MantineColorsTuple } from "@mantine/core";

// Resonite 公式パレット（wiki: Component:PlatformColorPalette / Branding guidelines）・dark 固定。
// 公式は各アクセント色が Hero(明=アクティブ) / Sub(暗=hover) / Dark(無効) の3階調 + Neutrals(Dark/Mid/Light)。
// 仕様書 §3.5 の値は Hero 相当。ここから Hero を中心に 10 段ランプを生成し、
//   shade[6]=Hero（primaryShade）/ [7..9]=Sub〜Dark（hover/押下/暗）/ [0..5]=明側ティント（light variant 用）
// とすることで hover・無効・light variant が正しく陰影付けされる。

function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace("#", "");
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}
function ch(n: number): string {
  return Math.round(Math.max(0, Math.min(255, n))).toString(16).padStart(2, "0");
}
function mix(a: string, b: string, t: number): string {
  const [ar, ag, ab] = hexToRgb(a);
  const [br, bg, bb] = hexToRgb(b);
  return `#${ch(ar + (br - ar) * t)}${ch(ag + (bg - ag) * t)}${ch(ab + (bb - ab) * t)}`;
}

// Hero hex から 10 段（明→暗・index 6 = Hero）を生成。
function ramp(hero: string): MantineColorsTuple {
  const W = "#ffffff";
  const B = "#0a0c10";
  return [
    mix(hero, W, 0.82),
    mix(hero, W, 0.64),
    mix(hero, W, 0.45),
    mix(hero, W, 0.28),
    mix(hero, W, 0.14),
    mix(hero, W, 0.05),
    hero, // 6 = Hero（primaryShade）
    mix(hero, B, 0.2), // 7 = hover（Sub 相当）
    mix(hero, B, 0.34), // 8
    mix(hero, B, 0.48), // 9 = Dark 相当
  ] as MantineColorsTuple;
}

// パレット外のサーフェス色（Resonite インスペクタ参照のカスタム暗色ティント）。
// 色の単一情報源としてここに集約（コンポーネントへ直書きしない）。
export const SURFACE = {
  sidebarBg: "#1a2a36", // dark cyan: サイドバー背景
  navActiveBg: "#2b2e26", // dark yellow: 選択タブ背景
} as const;

// 背景/サーフェス/テキスト（dark スキーム上書き）。
//   dark[7]=body 背景 / dark[6]=Paper/Card 既定背景 / dark[4]=既定ボーダー
//   dark[2]=dimmed/ラベル / dark[0]=本文
const dark: MantineColorsTuple = [
  "#e1e1e0", // 0 本文 (Light)
  "#c7c8c9", // 1
  "#86888b", // 2 ラベル/補助
  "#6a6d72", // 3
  "#3a3f47", // 4 ボーダー
  "#33383f", // 5
  "#2b2f35", // 6 カード/サイド (Mid)
  "#11151d", // 7 背景 (Dark)
  "#0d1018", // 8
  "#080b11", // 9
];

export const theme = createTheme({
  primaryColor: "brand",
  primaryShade: 6,
  defaultRadius: "md",
  // 明色アクセント（cyan/green/yellow 等）の filled 背景で文字を自動的に濃色化する。
  // これにより各所の「ラベルを濃色に」アドホック手当てを廃し、コントラストの単一情報源とする。
  // 危険ボタン（filled red）は白文字を保つため呼び出し側で autoContrast={false} を指定（ConfirmModal）。
  autoContrast: true,
  // 日本語フォールバックを明示（Win/各OSのシステムフォント優先）。
  fontFamily: 'system-ui, "Segoe UI", "Yu Gothic UI", "Hiragino Kaku Gothic ProN", "Meiryo", sans-serif',
  colors: {
    brand: ramp("#61d1fa"), // Cyan = 主アクション/選択
    dark,
    green: ramp("#59eb5c"), // 状態: 稼働中/有効/成功
    yellow: ramp("#f8f770"), // 注意/遷移中・選択タブ
    orange: ramp("#e69e50"), // 通知/強調
    red: ramp("#ff7676"), // 危険/破壊/エラー
    grape: ramp("#ba64f2"), // 二次/上級
  },
  components: {
    // ボタンは Mid (#2b2f35=dark[6]) 基準の控えめな面に（Resonite インスペクタ風）。
    // variant="default" は dark スキームで dark[6] 背景＝Mid。縁取りは外す。
    Button: { defaultProps: { variant: "default" }, styles: { root: { border: "none" } } },
    // ActionIcon（⋮ 等）も Mid 基準・縁取りなしに（既定で見えるように。subtle だとホバーまで透明）。
    ActionIcon: { defaultProps: { variant: "default" }, styles: { root: { border: "none" } } },
    // Select はテキスト入力欄ではないため縁取りを外す（縁取りは TextInput/PasswordInput のみ）。
    Select: { styles: { input: { border: "none" } } },
  },
});
