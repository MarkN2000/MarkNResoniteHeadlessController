import { useTranslation } from "react-i18next";
import { Divider, Stack, Switch } from "@mantine/core";
import type { RestartAnnounce, RestartConfig, RestartSessionChanges } from "../../api";
import { InspectorCard, FieldRow, InspectorTextInput, InspectorTextarea } from "../../components/inspector";

// ⑤事前アクションカード（§3.16(2)(7)）。告知（dynamicImpulse・フル設定型）＋セッション変更。
// 各サブ項目は独立トグル。無効時は関連フィールドを disabled にして関係を明示。
// 他カードと同じ value/onChange の1組で受け、preActions スライスの合成はカード内に閉じる。
export function PreActionsCard({
  value,
  onChange,
}: {
  value: RestartConfig["preActions"];
  onChange: (v: RestartConfig["preActions"]) => void;
}) {
  const { t } = useTranslation();
  const a = value.announce;
  const s = value.sessionChanges;
  const setAnnounce = (announce: RestartAnnounce) => onChange({ ...value, announce });
  const setSession = (sessionChanges: RestartSessionChanges) => onChange({ ...value, sessionChanges });

  return (
    <InspectorCard title={t("schedule.preActionsTitle")}>
      <Stack gap="xs">
        <Divider color="dark.4" label={t("schedule.announceSection")} labelPosition="center" />
        <FieldRow label={t("schedule.enabled")}>
          <Switch checked={a.enabled} onChange={(e) => setAnnounce({ ...a, enabled: e.currentTarget.checked })} />
        </FieldRow>
        <FieldRow label={t("schedule.announceItemUrl")}>
          <InspectorTextInput
            value={a.itemUrl}
            onChange={(e) => setAnnounce({ ...a, itemUrl: e.currentTarget.value })}
            placeholder="resrec:///..."
            disabled={!a.enabled}
          />
        </FieldRow>
        <FieldRow label={t("schedule.announceImpulseTag")}>
          <InspectorTextInput
            value={a.impulseTag}
            onChange={(e) => setAnnounce({ ...a, impulseTag: e.currentTarget.value })}
            placeholder="MRHC.play"
            disabled={!a.enabled}
          />
        </FieldRow>
        <FieldRow label={t("schedule.announceMessage")} align="start">
          <InspectorTextarea
            value={a.message}
            onChange={(e) => setAnnounce({ ...a, message: e.currentTarget.value })}
            disabled={!a.enabled}
          />
        </FieldRow>

        <Divider color="dark.4" label={t("schedule.sessionSection")} labelPosition="center" mt="xs" />
        <FieldRow label={t("schedule.setPrivate")}>
          <Switch
            checked={s.setPrivate}
            onChange={(e) => setSession({ ...s, setPrivate: e.currentTarget.checked })}
          />
        </FieldRow>
        <FieldRow label={t("schedule.setMaxUsersOne")}>
          <Switch
            checked={s.setMaxUsersOne}
            onChange={(e) => setSession({ ...s, setMaxUsersOne: e.currentTarget.checked })}
          />
        </FieldRow>
        <FieldRow label={t("schedule.renameEnabled")}>
          <Switch
            checked={s.renameEnabled}
            onChange={(e) => setSession({ ...s, renameEnabled: e.currentTarget.checked })}
          />
        </FieldRow>
        <FieldRow label={t("schedule.renameTo")}>
          <InspectorTextInput
            value={s.renameTo}
            onChange={(e) => setSession({ ...s, renameTo: e.currentTarget.value })}
            disabled={!s.renameEnabled}
          />
        </FieldRow>
      </Stack>
    </InspectorCard>
  );
}
