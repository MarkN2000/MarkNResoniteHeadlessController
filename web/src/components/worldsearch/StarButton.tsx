import { ActionIcon } from "@mantine/core";

// ★/☆ のコンパクトなトグルアイコン（RefreshButton と同じ ActionIcon 規約）。
// active=登録済/表示中＝塗り★(黄)、未＝枠☆(灰)。絵文字なので aria-label/title 必須。
// StartPanel（URL欄）・WorldSearchView（検索・新規セッション/コンフィグ編集共用）で共有
// （UI改善②で components/worldsearch へ移設）。disabled=お気に入り不可URL等で無効化。
export function StarButton({
  active,
  onClick,
  label,
  disabled = false,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  disabled?: boolean;
}) {
  return (
    <ActionIcon
      variant="light"
      color={active ? "yellow" : "gray"}
      size="lg"
      radius="md"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      title={label}
    >
      <span style={{ fontSize: 18, color: active ? "var(--mantine-color-yellow-4)" : "var(--mantine-color-dark-0)" }}>
        {active ? "★" : "☆"}
      </span>
    </ActionIcon>
  );
}
