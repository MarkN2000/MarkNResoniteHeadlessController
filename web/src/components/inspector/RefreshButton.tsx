import { ActionIcon } from "@mantine/core";

// カードヘッダ右に置く「更新（再取得）」アイコンボタン（くるっと回った矢印 ⟳）。
// 参考画像の複製/削除アイコンと同じ位置・サイズ。各タブのヘッダで使い回す。
export function RefreshButton({
  onClick,
  loading,
  label,
}: {
  onClick: () => void;
  loading?: boolean;
  label: string;
}) {
  return (
    <ActionIcon
      variant="light"
      color="gray"
      size="lg"
      radius="md"
      onClick={onClick}
      loading={loading}
      aria-label={label}
      title={label}
    >
      <span style={{ fontSize: 18, color: "var(--mantine-color-dark-0)" }}>⟳</span>
    </ActionIcon>
  );
}
