import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Checkbox, Divider, Group, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import {
  FieldRow,
  InspectorButton,
  InspectorCard,
  InspectorSelect,
  InspectorTextInput,
} from "../../components/inspector";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useItemTemplates } from "../../hooks/useItemTemplates";
import { MANUAL_TEMPLATE, buildTemplateSelectData } from "../../lib/itemTemplates";
import { isResoniteUrl } from "../../lib/resoniteUrl";

// スポーン / インパルス（R14・フォーカス中セッションへ）。
//   アイテムスポーン       = spawn "<url>" <active> <persistent>（テンプレ選択 or 手動 URL・2026-06-10）
//   ダイナミックインパルス = dynamicimpulsestring "<tag>" "<value>"（tag 必須・value 任意）
//   スポーン＆パルス       = テンプレ（リモートリスト）or 手動 → backend が spawn→約5秒→impulse を完走
//                            （告知③のセッション版・docs/design/announce-templates.md）
// いずれも非破壊操作なので確認ダイアログなし＝実行→受理トースト（方針A・respawn/message と同格）。
// spawn/impulse はセッションの users/status を変えない（再取得不要）ため onChanged は受け取らない。
export function SpawnImpulseCard({ idx }: { idx: number }) {
  const { t, i18n } = useTranslation();
  const { busy, run } = useAsyncAction();
  const [url, setUrl] = useState("");
  const [active, setActive] = useState(true);
  const [persistent, setPersistent] = useState(false);
  const [tag, setTag] = useState("");
  const [value, setValue] = useState("");

  const urlValid = isResoniteUrl(url);
  const tagValid = tag.trim() !== "";

  // アイテムスポーン単体のテンプレ選択（未操作なら先頭テンプレ・専用リスト=tag任意・2026-06-10）。
  // テンプレの url だけを使い tag は使わない。active/persistent は選択と独立に効く
  // （単体スポーンの存在意義なので残す）。取得前は手動入力に退化。
  const spawnTemplates = useItemTemplates(api.getItemSpawnTemplates);
  const [spawnSel, setSpawnSel] = useState<string | null>(null);
  const spawnKey = spawnSel ?? spawnTemplates[0]?.id ?? MANUAL_TEMPLATE;
  const spawnManual = spawnKey === MANUAL_TEMPLATE;
  const spawnData = buildTemplateSelectData(
    spawnTemplates,
    spawnManual ? "" : spawnKey,
    i18n.language,
    t("session.spawnPulseManual"),
  );
  const spawnUrl = spawnManual ? url.trim() : (spawnTemplates.find((tpl) => tpl.id === spawnKey)?.url ?? "");
  const spawnReady = spawnManual ? urlValid : spawnUrl !== "";

  // スポーン＆パルス（専用リスト・tag必須）。選択は未操作なら先頭テンプレを既定にする。
  const templates = useItemTemplates(api.getSpawnTemplates);
  const [spSel, setSpSel] = useState<string | null>(null);
  const [spUrl, setSpUrl] = useState("");
  const [spTag, setSpTag] = useState("");
  const [spMessage, setSpMessage] = useState("");
  const spKey = spSel ?? templates[0]?.id ?? MANUAL_TEMPLATE;
  const spManual = spKey === MANUAL_TEMPLATE;
  const spData = buildTemplateSelectData(
    templates,
    spManual ? "" : spKey,
    i18n.language,
    t("session.spawnPulseManual"),
  );
  // 手動時のみ url/tag を検証（url は空可＝spawn 省略で impulse のみ・告知③と同条件）。
  const spReady = !spManual || (spTag.trim() !== "" && (spUrl.trim() === "" || isResoniteUrl(spUrl)));
  const runSpawnPulse = () =>
    void run(
      () =>
        api.spawnImpulse(
          idx,
          spManual
            ? { itemUrl: spUrl.trim(), impulseTag: spTag.trim(), message: spMessage }
            : { templateId: spKey, message: spMessage },
        ),
      t("toast.spawnPulseDone"),
    );

  return (
    <InspectorCard title={t("session.spawnImpulse")}>
      <Stack gap={10}>
        {/* アイテムスポーン（テンプレ選択 or 手動 URL。スポーン＆パルスと同パターン） */}
        <Text size="xs" fw={700} c="dimmed">
          {t("session.spawnSection")}
        </Text>
        <FieldRow label={t("session.spawnPulseItem")}>
          <InspectorSelect data={spawnData} value={spawnKey} onChange={(v) => v && setSpawnSel(v)} />
        </FieldRow>
        {spawnManual && (
          <FieldRow label={t("session.spawnUrl")} align="start">
            <Stack gap={4}>
              <InspectorTextInput
                value={url}
                onChange={(e) => setUrl(e.currentTarget.value)}
                placeholder={t("session.spawnUrlPlaceholder")}
              />
              {url.trim() !== "" && !urlValid && (
                <Text size="xs" c="dimmed">
                  {t("session.spawnUrlHint")}
                </Text>
              )}
            </Stack>
          </FieldRow>
        )}
        <Group justify="space-between" wrap="wrap" gap="xs">
          <Group gap="md">
            <Checkbox
              size="xs"
              label={t("session.spawnActive")}
              checked={active}
              onChange={(e) => setActive(e.currentTarget.checked)}
            />
            <Checkbox
              size="xs"
              label={t("session.spawnPersistent")}
              checked={persistent}
              onChange={(e) => setPersistent(e.currentTarget.checked)}
            />
          </Group>
          <InspectorButton
            disabled={busy || !spawnReady}
            onClick={() => void run(() => api.spawnItem(idx, spawnUrl, active, persistent), t("toast.spawnDone"))}
          >
            {t("session.spawn")}
          </InspectorButton>
        </Group>

        <Divider my={2} color="dark.4" />

        {/* ダイナミックインパルス */}
        <Text size="xs" fw={700} c="dimmed">
          {t("session.impulseSection")}
        </Text>
        <FieldRow label={t("session.impulseTag")}>
          <InspectorTextInput
            value={tag}
            onChange={(e) => setTag(e.currentTarget.value)}
            placeholder={t("session.impulseTagPlaceholder")}
          />
        </FieldRow>
        <FieldRow label={t("session.impulseValue")}>
          <InspectorTextInput
            value={value}
            onChange={(e) => setValue(e.currentTarget.value)}
            placeholder={t("session.impulseValuePlaceholder")}
          />
        </FieldRow>
        <Group justify="flex-end">
          <InspectorButton
            disabled={busy || !tagValid}
            onClick={() => void run(() => api.sendImpulse(idx, tag.trim(), value), t("toast.impulseDone"))}
          >
            {t("session.impulseSend")}
          </InspectorButton>
        </Group>

        <Divider my={2} color="dark.4" />

        {/* スポーン＆パルス（実行ボタンは backend 完走まで約5秒+α busy のまま） */}
        <Text size="xs" fw={700} c="dimmed">
          {t("session.spawnPulseSection")}
        </Text>
        <FieldRow label={t("session.spawnPulseItem")}>
          <InspectorSelect data={spData} value={spKey} onChange={(v) => v && setSpSel(v)} />
        </FieldRow>
        {spManual && (
          <>
            <FieldRow label={t("session.spawnUrl")} align="start">
              <Stack gap={4}>
                <InspectorTextInput
                  value={spUrl}
                  onChange={(e) => setSpUrl(e.currentTarget.value)}
                  placeholder={t("session.spawnUrlPlaceholder")}
                />
                {spUrl.trim() !== "" && !isResoniteUrl(spUrl) && (
                  <Text size="xs" c="dimmed">
                    {t("session.spawnUrlHint")}
                  </Text>
                )}
              </Stack>
            </FieldRow>
            <FieldRow label={t("session.impulseTag")}>
              <InspectorTextInput
                value={spTag}
                onChange={(e) => setSpTag(e.currentTarget.value)}
                placeholder={t("session.impulseTagPlaceholder")}
              />
            </FieldRow>
          </>
        )}
        <FieldRow label={t("session.spawnPulseMessage")}>
          <InspectorTextInput
            value={spMessage}
            onChange={(e) => setSpMessage(e.currentTarget.value)}
            placeholder={t("session.impulseValuePlaceholder")}
          />
        </FieldRow>
        <Group justify="flex-end">
          <InspectorButton disabled={busy || !spReady} onClick={runSpawnPulse}>
            {t("session.spawnPulseRun")}
          </InspectorButton>
        </Group>
      </Stack>
    </InspectorCard>
  );
}
