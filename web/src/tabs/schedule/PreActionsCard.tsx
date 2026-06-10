import { useTranslation } from "react-i18next";
import { Divider, Stack, Switch } from "@mantine/core";
import * as api from "../../api";
import type { RestartAnnounce, RestartConfig, RestartSessionChanges } from "../../api";
import {
  InspectorCard,
  FieldRow,
  InspectorSelect,
  InspectorTextInput,
  InspectorTextarea,
} from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import { useItemTemplates } from "../../hooks/useItemTemplates";
import { MANUAL_TEMPLATE, buildTemplateSelectData } from "../../lib/itemTemplates";
import { defaultAnnounce, defaultSessionChanges } from "./scheduleModel";

// ⑤事前アクションカード（§3.16(2)(7)）。告知（dynamicImpulse・フル設定型）＋セッション変更。
// 他カードと同じ value/onChange の1組で受け、preActions スライスの合成はカード内に閉じる。
// 告知 OFF・改名 OFF のときは配下の入力欄を非表示にして関係を明示する。
export function PreActionsCard({
  value,
  onChange,
}: {
  value: RestartConfig["preActions"];
  onChange: (v: RestartConfig["preActions"]) => void;
}) {
  const { t, i18n } = useTranslation();
  const a = value.announce;
  const s = value.sessionChanges;

  // 告知テンプレ一覧（backend がリモートリスト＋フォールバックで常に何かしら返す）。
  const templates = useItemTemplates(api.getAnnounceTemplates);
  const setAnnounce = (announce: RestartAnnounce) => onChange({ ...value, announce });
  const setSession = (sessionChanges: RestartSessionChanges) => onChange({ ...value, sessionChanges });

  // マーカークリック＝その項目を既定値へ戻す（確認あり）。
  const confirm = useConfirm();
  const resetProps = (apply: () => void, fieldLabel: string) => ({
    markerLabel: t("common.resetToDefault"),
    onMarkerClick: () =>
      confirm.ask({
        title: t("common.resetConfirmTitle"),
        message: t("common.resetConfirmMsg", { field: fieldLabel }),
        onConfirm: apply,
      }),
  });

  // 告知アイテム＝テンプレ選択 or 手動入力。保存値（templateId）から選択状態を導出する
  // （選択肢の組み立て＝消滅id補完含む は lib/itemTemplates と共用）。
  const itemKey = a.templateId !== "" ? a.templateId : MANUAL_TEMPLATE;
  const isManual = itemKey === MANUAL_TEMPLATE;
  const itemData = buildTemplateSelectData(templates, a.templateId, i18n.language, t("schedule.announceManual"));

  // テンプレ選択＝templateId のみ保存（URL/タグは backend が告知時に解決）。
  // 手動＝templateId を空にして itemUrl/impulseTag の入力欄を開く。
  const selectItem = (v: string | null) => {
    if (!v) return;
    if (v === MANUAL_TEMPLATE) setAnnounce({ ...a, templateId: "" });
    else setAnnounce({ ...a, templateId: v, itemUrl: "", impulseTag: "" });
  };

  return (
    <InspectorCard title={t("schedule.preActionsTitle")}>
      <Stack gap="xs">
        <Divider color="dark.4" label={t("schedule.announceSection")} labelPosition="center" />
        <FieldRow
          label={t("schedule.enabled")}
          {...resetProps(() => setAnnounce({ ...a, enabled: defaultAnnounce().enabled }), t("schedule.enabled"))}
        >
          <Switch checked={a.enabled} onChange={(e) => setAnnounce({ ...a, enabled: e.currentTarget.checked })} />
        </FieldRow>
        {a.enabled && (
          <>
            {/* 告知アイテム種別＝templateId（テンプレ）/ 空（手動）の選択。既定値はテンプレ自体なのでリセット対象外。 */}
            <FieldRow label={t("schedule.announceItemType")}>
              <InspectorSelect data={itemData} value={itemKey} onChange={selectItem} />
            </FieldRow>
            {isManual && (
              <>
                <FieldRow
                  label={t("schedule.announceItemUrl")}
                  {...resetProps(() => setAnnounce({ ...a, itemUrl: defaultAnnounce().itemUrl }), t("schedule.announceItemUrl"))}
                >
                  <InspectorTextInput
                    value={a.itemUrl}
                    onChange={(e) => setAnnounce({ ...a, itemUrl: e.currentTarget.value })}
                    placeholder="resrec:///..."
                  />
                </FieldRow>
                <FieldRow
                  label={t("schedule.announceImpulseTag")}
                  {...resetProps(
                    () => setAnnounce({ ...a, impulseTag: defaultAnnounce().impulseTag }),
                    t("schedule.announceImpulseTag"),
                  )}
                >
                  <InspectorTextInput
                    value={a.impulseTag}
                    onChange={(e) => setAnnounce({ ...a, impulseTag: e.currentTarget.value })}
                    placeholder="MRHC.play"
                  />
                </FieldRow>
              </>
            )}
            <FieldRow
              label={t("schedule.announceMessage")}
              align="start"
              {...resetProps(() => setAnnounce({ ...a, message: defaultAnnounce().message }), t("schedule.announceMessage"))}
            >
              <InspectorTextarea
                value={a.message}
                onChange={(e) => setAnnounce({ ...a, message: e.currentTarget.value })}
              />
            </FieldRow>
          </>
        )}

        <Divider color="dark.4" label={t("schedule.sessionSection")} labelPosition="center" mt="xs" />
        <FieldRow
          label={t("schedule.setPrivate")}
          {...resetProps(() => setSession({ ...s, setPrivate: defaultSessionChanges().setPrivate }), t("schedule.setPrivate"))}
        >
          <Switch
            checked={s.setPrivate}
            onChange={(e) => setSession({ ...s, setPrivate: e.currentTarget.checked })}
          />
        </FieldRow>
        <FieldRow
          label={t("schedule.setMaxUsersOne")}
          {...resetProps(
            () => setSession({ ...s, setMaxUsersOne: defaultSessionChanges().setMaxUsersOne }),
            t("schedule.setMaxUsersOne"),
          )}
        >
          <Switch
            checked={s.setMaxUsersOne}
            onChange={(e) => setSession({ ...s, setMaxUsersOne: e.currentTarget.checked })}
          />
        </FieldRow>
        <FieldRow
          label={t("schedule.renameEnabled")}
          {...resetProps(
            () => setSession({ ...s, renameEnabled: defaultSessionChanges().renameEnabled }),
            t("schedule.renameEnabled"),
          )}
        >
          <Switch
            checked={s.renameEnabled}
            onChange={(e) => setSession({ ...s, renameEnabled: e.currentTarget.checked })}
          />
        </FieldRow>
        {s.renameEnabled && (
          <FieldRow
            label={t("schedule.renameTo")}
            {...resetProps(() => setSession({ ...s, renameTo: defaultSessionChanges().renameTo }), t("schedule.renameTo"))}
          >
            <InspectorTextInput
              value={s.renameTo}
              onChange={(e) => setSession({ ...s, renameTo: e.currentTarget.value })}
            />
          </FieldRow>
        )}
      </Stack>
      <ConfirmHost confirm={confirm} />
    </InspectorCard>
  );
}
