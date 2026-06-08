// タブ定義（状態ベースのナビ・text-only）。docs/design/phase-7-spec.md §3.3。

export type TabId =
  | "session"
  | "friends"
  | "newSession"
  | "config"
  | "schedule"
  | "settings"
  | "command"
  | "logs";

export interface TabDef {
  id: TabId;
  labelKey: string; // i18n キー
  // 停止中でも使えるか（config/schedule/settings はファイル編集系なので true、
  // session/friends/newSession/command はヘッドレス稼働が要るため false）。
  availableWhenStopped: boolean;
}

export const TABS: TabDef[] = [
  { id: "session", labelKey: "tabs.session", availableWhenStopped: false },
  { id: "friends", labelKey: "tabs.friends", availableWhenStopped: false },
  { id: "newSession", labelKey: "tabs.newSession", availableWhenStopped: false },
  { id: "config", labelKey: "tabs.config", availableWhenStopped: true },
  { id: "schedule", labelKey: "tabs.schedule", availableWhenStopped: true },
  { id: "settings", labelKey: "tabs.settings", availableWhenStopped: true },
  { id: "command", labelKey: "tabs.command", availableWhenStopped: false },
  // ログ閲覧はディスク上のログファイルを読むだけなので停止中でも使える（クラッシュ後の診断に有用）。
  { id: "logs", labelKey: "tabs.logs", availableWhenStopped: true },
];
