import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, ScrollArea, Stack } from "@mantine/core";
import * as api from "../../api";
import type { ConfigSummary, RestartStatus } from "../../api";
import { SplitColumns } from "../../components/SplitColumns";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useConfirm } from "../../hooks/useConfirm";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";
import { StatusCard } from "./StatusCard";
import { ManualCard } from "./ManualCard";

// スケジュール（自動再起動）タブ（Phase 8・§3.16(7)）。停止中でも設定編集可（手動再起動のみ稼働中）。
// レイアウト: SplitColumns 左=運用/状態〔①②③〕 右=設定〔④⑤⑥〕。
// P8-5a は ①状態（poll）＋②手動再起動。③予定リスト・④⑤⑥設定は P8-5b。
export function ScheduleTab({ running, configs }: { running: boolean; configs: ConfigSummary[] }) {
  const { t } = useTranslation();
  const [rs, setRs] = useState<RestartStatus | null>(null);
  const confirm = useConfirm();

  const refetch = useCallback(async () => {
    setRs(await api.getRestartStatus());
  }, []);
  useEffect(() => {
    void refetch();
  }, [refetch]);
  // 進行中フェーズ/残り時間を追従するため短め（3秒）にポーリング（表示中のみ）。
  useVisiblePolling(refetch, 3000);

  const onRestart = (configName: string) => {
    confirm.ask({
      title: t("schedule.confirmRestartTitle"),
      message: t("schedule.confirmRestart"),
      success: t("schedule.toastRestartAccepted"),
      onConfirm: async () => {
        const r = await api.triggerRestart(configName);
        void refetch();
        return r;
      },
    });
  };

  const onCancel = () => {
    confirm.ask({
      title: t("schedule.cancelTitle"),
      message: `${t("schedule.confirmCancel")}\n${t("schedule.cancelNote")}`,
      danger: true,
      success: t("schedule.toastCancelDone"),
      onConfirm: async () => {
        const r = await api.cancelRestart();
        void refetch();
        return r;
      },
    });
  };

  return (
    <>
      <ScrollArea h="100%" type="hover">
        <Box pb="md">
          <SplitColumns
            left={
              <Stack gap="lg">
                <StatusCard status={rs} running={running} onCancel={onCancel} />
                <ManualCard running={running} configs={configs} onRestart={onRestart} />
              </Stack>
            }
            right={<Stack gap="lg">{/* P8-5b: ③予定リスト・④待機制御・⑤事前アクション・⑥クラッシュ復帰 */}</Stack>}
          />
        </Box>
      </ScrollArea>

      <ConfirmModal
        opened={confirm.request !== null}
        title={confirm.request?.title ?? ""}
        message={confirm.request?.message}
        danger={confirm.request?.danger}
        loading={confirm.busy}
        onConfirm={() => void confirm.confirm()}
        onClose={confirm.close}
      />
    </>
  );
}
