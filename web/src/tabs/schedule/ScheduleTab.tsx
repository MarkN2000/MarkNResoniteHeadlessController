import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Center, Loader, ScrollArea, Stack } from "@mantine/core";
import * as api from "../../api";
import type { ConfigSummary, ItemTemplate, RestartConfig, RestartStatus, ScheduledRestart, WriteResult } from "../../api";
import { SplitColumns } from "../../components/SplitColumns";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useTtsSpeakers } from "../../hooks/useTtsSpeakers";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";
import { StatusCard } from "./StatusCard";
import { ManualCard } from "./ManualCard";
import { SystemMetricsCard } from "./SystemMetricsCard";
import { ScheduleListCard } from "./ScheduleListCard";
import { WaitControlCard } from "./WaitControlCard";
import { PreActionsCard } from "./PreActionsCard";
import { CrashRecoveryCard } from "./CrashRecoveryCard";
import { UpdateOnRestartCard } from "./UpdateOnRestartCard";
import { SaveBar } from "./SaveBar";

// スケジュール（自動再起動）タブ（Phase 8・§3.16(7)）。停止中でも設定編集可（手動再起動のみ稼働中）。
// レイアウト: SplitColumns 左=運用/状態〔①②〕 右=設定〔③④⑤⑥〕＋一括保存バー。
// ①状態・②手動は live（poll＋アクション）。③④⑤⑥は単一 working＋dirty 判定で完全オブジェクト PUT。
export function ScheduleTab({
  running,
  configs,
  templates,
}: {
  running: boolean;
  configs: ConfigSummary[];
  templates: ItemTemplate[];
}) {
  const { t } = useTranslation();
  const [rs, setRs] = useState<RestartStatus | null>(null);
  const [rc, setRc] = useState<RestartConfig | null>(null);
  const [original, setOriginal] = useState<RestartConfig | null>(null);
  const confirm = useConfirm();
  const apply = useAsyncAction();
  const announceTemplates = templates.filter((template) => template.actions.includes("announce"));
  const announce = rc?.preActions.announce;
  const announceTemplate = announceTemplates.find((template) => template.id === announce?.templateId);
  // テンプレート一覧の取得前でも、保存済み speakerId があれば TTS 設定として扱って保存を待たせる。
  const isTtsAnnouncement =
    announce?.enabled === true &&
    (announceTemplate?.input?.kind === "ttsVoice" || (announceTemplate === undefined && announce.speakerId > 0));
  const ttsSpeakers = useTtsSpeakers(isTtsAnnouncement);

  // working≠original で dirty 判定（コンフィグタブと同方式・キー順は in-place 編集で保持）。
  const dirty = rc !== null && original !== null && JSON.stringify(rc) !== JSON.stringify(original);
  const ttsAnnouncementValid =
    !isTtsAnnouncement ||
    (announce !== undefined &&
      announce.message.trim() !== "" &&
      announce.speakerId !== 0 &&
      !ttsSpeakers.loading &&
      !ttsSpeakers.failed &&
      ttsSpeakers.voices.some((voice) => voice.id === announce.speakerId));

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

  const onStop = () => {
    confirm.ask({
      title: t("schedule.waitThenStop"),
      message: t("schedule.confirmStop"),
      danger: true,
      success: t("schedule.toastStopAccepted"),
      onConfirm: async () => {
        const r = await api.gracefulStop();
        void refetch();
        return r;
      },
    });
  };

  const onCancel = () => {
    // 進行中が通常停止（R7）なら中止ダイアログ文言を「停止」連動にする（既定は再起動）。
    const isStop = rs?.restartTriggerType === "stop";
    confirm.ask({
      title: t(isStop ? "schedule.cancelStopTitle" : "schedule.cancelTitle"),
      message: `${t(isStop ? "schedule.confirmCancelStop" : "schedule.confirmCancel")}\n${t("schedule.cancelNote")}`,
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

  // 予定リストはその場で即保存（一括「保存」から分離）。設定値は最後に保存した値(original)を使うので、
  // 未保存の待機/事前アクション/クラッシュ復帰の編集は巻き込まない（それらは従来どおり一括保存）。
  // トーストは呼び出し側（ScheduleListCard の useAsyncAction / useConfirm）が1回だけ出す（ここでは出さない）。
  const persist = async (scheduled: ScheduledRestart[]): Promise<WriteResult> => {
    if (!rc || !original) return { ok: true };
    const persisted = { ...original, scheduled };
    const r = await api.putRestartConfig(persisted);
    if (r.ok) {
      setOriginal(persisted);
      setRc((cur) => (cur ? { ...cur, scheduled } : cur));
      void refetch();
    }
    return r;
  };

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
                <SystemMetricsCard />
                <ManualCard running={running} configs={configs} onRestart={onRestart} onStop={onStop} />
              </Stack>
            }
            right={
              rc ? (
                <Stack gap="lg">
                  <ScheduleListCard schedules={rc.scheduled} configs={configs} onPersist={persist} />
                  <WaitControlCard value={rc.waitControl} onChange={(waitControl) => patch({ waitControl })} />
                  <PreActionsCard
                    value={rc.preActions}
                    onChange={(preActions) => patch({ preActions })}
                    templates={announceTemplates}
                    ttsSpeakers={ttsSpeakers}
                  />
                  <CrashRecoveryCard value={rc.crashRecovery} onChange={(crashRecovery) => patch({ crashRecovery })} />
                  <UpdateOnRestartCard
                    scheduled={rc.updateOnScheduledRestart}
                    manual={rc.updateBeforeManualStart}
                    onScheduledChange={(updateOnScheduledRestart) => patch({ updateOnScheduledRestart })}
                    onManualChange={(updateBeforeManualStart) => patch({ updateBeforeManualStart })}
                  />
                  <SaveBar dirty={dirty} valid={ttsAnnouncementValid} saving={apply.busy} onSave={save} />
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

      <ConfirmHost confirm={confirm} />
    </>
  );
}
