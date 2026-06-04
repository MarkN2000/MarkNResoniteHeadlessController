import { ActionIcon } from "@mantine/core";

// 行リスト末尾の「＋追加」アイコンボタン（RowsEditor / AdvancedFieldsEditor 共通）。
// size=lg・variant="light"・gray・＋ に固定し、ラベルと onClick・disabled だけ可変。
export function AddIconButton({ label, onClick, disabled }: { label: string; onClick: () => void; disabled?: boolean }) {
  return (
    <ActionIcon size="lg" variant="light" color="gray" aria-label={label} title={label} disabled={disabled} onClick={onClick}>
      ＋
    </ActionIcon>
  );
}
