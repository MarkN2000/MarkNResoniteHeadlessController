import { Button } from "@mantine/core";
import type { ButtonProps, ElementProps } from "@mantine/core";

// 「選択中」状態を表すボタンの共通コンポーネント（選択ハイライト色の単一情報源）。
// 選択中 = brand（Resonite Cyan）filled。文字は theme の autoContrast で自動濃色化（既存規約）。
// 非選択 = default（Mid グレー面・variant="default" は color 非依存）。
// 旧来の「選択中 = filled gray」は明るいグレーで視認しづらかったため brand に統一した。
// 適用先: コンフィグ一覧の行・ワールドタブ・Friends ソース切替。
interface SelectionButtonProps extends ButtonProps, ElementProps<"button", keyof ButtonProps> {
  selected: boolean;
}

export function SelectionButton({ selected, ...props }: SelectionButtonProps) {
  return <Button size="xs" variant={selected ? "filled" : "default"} color="brand" {...props} />;
}
