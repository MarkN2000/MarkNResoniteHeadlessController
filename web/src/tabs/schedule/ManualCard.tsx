import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Stack, Text } from "@mantine/core";
import type { ConfigSummary } from "../../api";
import { InspectorCard, FieldRow, InspectorSelect, InspectorButton } from "../../components/inspector";

// Select は空文字値を扱えないため番兵を使い、送信時に "" (=前回config) へ変換する。
// 番兵は config 名として無効な文字（"#"）を含むため、実在 config 名（[A-Za-z0-9_-]）と衝突しない。
const PREV = "#prev";

// ②手動カード（§3.16(7)）。通常（安全）再起動を config 選択付きで受け付ける。稼働中のみ有効。
// 実行確認は親（ScheduleTab）の useConfirm が担当（onRestart に configName を渡す）。
export function ManualCard({
  running,
  configs,
  onRestart,
}: {
  running: boolean;
  configs: ConfigSummary[];
  onRestart: (configName: string) => void;
}) {
  const { t } = useTranslation();
  const [sel, setSel] = useState<string>(PREV);

  const data = [
    { value: PREV, label: t("schedule.usePrevious") },
    ...configs.map((c) => ({ value: c.name, label: c.name })),
  ];

  return (
    <InspectorCard title={t("schedule.manualTitle")}>
      <Stack gap="xs">
        <FieldRow label={t("schedule.restartConfig")}>
          <InspectorSelect data={data} value={sel} onChange={(v) => setSel(v ?? PREV)} />
        </FieldRow>
        <InspectorButton
          fullWidth
          disabled={!running}
          onClick={() => onRestart(sel === PREV ? "" : sel)}
        >
          {t("schedule.normalRestart")}
        </InspectorButton>
        {!running && (
          <Text size="xs" c="dimmed" ta="center">
            {t("schedule.onlyWhenRunning")}
          </Text>
        )}
      </Stack>
    </InspectorCard>
  );
}
