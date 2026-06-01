import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Center, Loader, ScrollArea, Stack } from "@mantine/core";
import * as api from "../../api";
import type { ConfigSummary, RestartConfig, RestartStatus } from "../../api";
import { SplitColumns } from "../../components/SplitColumns";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useConfirm } from "../../hooks/useConfirm";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";
import { StatusCard } from "./StatusCard";
import { ManualCard } from "./ManualCard";
import { ScheduleListCard } from "./ScheduleListCard";
import { WaitControlCard } from "./WaitControlCard";
import { PreActionsCard } from "./PreActionsCard";
import { CrashRecoveryCard } from "./CrashRecoveryCard";
import { SaveBar } from "./SaveBar";

// スケジュール（自動再起動）タブ（Phase 8・§3.16(7)）。停止中でも設定編集可（手動再起動のみ稼働中）。
// レイアウト: SplitColumns 左=運用/状態〔①②〕 右=設定〔③④⑤⑥〕＋一括保存バー。
// ①状態・②手動は live（poll＋アクション）。③④⑤⑥は単一 working＋dirty 判定で完全オブジェクト PUT。
export function ScheduleTab({ running, configs }: { running: boolean; configs: ConfigSummary[] }) {
  const { t } = useTranslation();
  const [rs, setRs] = useState<RestartStatus | null>(null);
  const [rc, setRc] = useState<RestartConfig | null>(null);
  const [original, setOriginal] = useState<RestartConfig | null>(null);
  const confirm = useConfirm();
  const apply = useAsyncAction();

  // working≠original で dirty 判定（コンフィグタブと同方式・キー順は in-place 編集で保持）。
  const dirty = rc !== null && original !== null && JSON.stringify(rc) !== JSON.stringify(original);

  const refetch = useCallback(async () => {
    setRs(await api.getRestartStatus());
  }, []);
  useEffect(() => {
    void refetch();
    void (async () => {
      const c = await api.getRestartConfig();
      setRc(c);
      setOriginal(c);
    })();
  }, [refetch]);
  // 進行中フェーズ/残り時間を追従するため短め（3秒）にポーリング（表示中のみ）。設定(rc)は対象外。
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

  // 設定群の一括保存。完全オブジェクトを PUT（backend pointer 設計に一致）。
  // 保存後に next-schedule が変わるため status を refetch（backend 側でも scheduler.Reload 発火）。
  const save = () =>
    apply.run(async () => {
      if (!rc) return { ok: true };
      const r = await api.putRestartConfig(rc);
      if (r.ok) {
        setOriginal(rc);
        void refetch();
      }
      return r;
    }, t("schedule.toastSaved"));

  // rc 更新ヘルパ（スライス単位の onChange を合成）。
  const patch = (p: Partial<RestartConfig>) => rc && setRc({ ...rc, ...p });

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
            right={
              rc ? (
                <Stack gap="lg">
                  <ScheduleListCard
                    schedules={rc.scheduled}
                    configs={configs}
                    onChange={(scheduled) => patch({ scheduled })}
                  />
                  <WaitControlCard value={rc.waitControl} onChange={(waitControl) => patch({ waitControl })} />
                  <PreActionsCard value={rc.preActions} onChange={(preActions) => patch({ preActions })} />
                  <CrashRecoveryCard value={rc.crashRecovery} onChange={(crashRecovery) => patch({ crashRecovery })} />
                  <SaveBar dirty={dirty} saving={apply.busy} onSave={save} />
                </Stack>
              ) : (
                <Center h={200}>
                  <Loader />
                </Center>
              )
            }
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
