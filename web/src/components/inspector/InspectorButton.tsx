import { Button } from "@mantine/core";
import type { ButtonProps, ElementProps } from "@mantine/core";

// 重大度で色分けした light tint ボタン（中立=gray / 注意=yellow / 危険=red）。
// インスペクタ UI のアクションボタンを統一する。色の単一情報源。
export type Severity = "neutral" | "warning" | "danger";
const SEVERITY_COLOR: Record<Severity, string> = {
  neutral: "gray",
  warning: "yellow",
  danger: "red",
};

interface InspectorButtonProps extends ButtonProps, ElementProps<"button", keyof ButtonProps> {
  severity?: Severity;
}

export function InspectorButton({ severity = "neutral", ...props }: InspectorButtonProps) {
  return <Button size="xs" variant="light" color={SEVERITY_COLOR[severity]} {...props} />;
}
