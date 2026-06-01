import { Button } from "@mantine/core";
import { useTranslation } from "react-i18next";

// 設定群（③④⑤⑥）共通の一括保存バー（§3.16(7)・コンフィグタブと同方式）。
// dirty 時のみ有効。完全オブジェクト PUT は親（ScheduleTab）が担当。
export function SaveBar({ dirty, saving, onSave }: { dirty: boolean; saving: boolean; onSave: () => void }) {
  const { t } = useTranslation();
  return (
    <Button
      fullWidth
      size="xs"
      variant={dirty ? "filled" : "default"}
      color="brand"
      styles={dirty ? { label: { color: "var(--mantine-color-dark-9)" } } : undefined}
      disabled={!dirty}
      loading={saving}
      onClick={onSave}
    >
      {t("schedule.save")}
    </Button>
  );
}
