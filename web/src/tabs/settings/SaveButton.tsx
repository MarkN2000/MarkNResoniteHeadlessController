import { Button } from "@mantine/core";

// 設定タブ各セクションの主アクション（保存/変更）ボタン。
// session/config タブの「適用/保存」と同じ見た目（有効時のみ cyan filled・ラベル濃色）。§3.8。
export function SaveButton({
  label,
  onClick,
  disabled,
  loading,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  loading?: boolean;
}) {
  return (
    <Button
      fullWidth
      size="xs"
      mt={4}
      variant={disabled ? "default" : "filled"}
      color="brand"
      disabled={disabled}
      loading={loading}
      onClick={onClick}
    >
      {label}
    </Button>
  );
}
