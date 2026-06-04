import type { ReactNode } from "react";
import { ActionIcon } from "@mantine/core";
import { ROW_ICON_SIZE } from "./FieldRow";

// 一覧/行の小さなアクションアイコン（削除×・複製⧉・編集✎ 等）の共通ボタン。
// size=ROW_ICON_SIZE・variant="light"（淡い色背景で視認できる）に固定し、色/記号/ラベルだけ可変にする。
// variant="subtle"（背景透明）だと暗背景に同化して見えづらいため light に統一する（ConfigList/ScheduleListCard 基準）。
export function RowIconButton({
  color,
  label,
  onClick,
  disabled,
  children,
}: {
  color: string;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <ActionIcon
      size={ROW_ICON_SIZE}
      variant="light"
      color={color}
      title={label}
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </ActionIcon>
  );
}
