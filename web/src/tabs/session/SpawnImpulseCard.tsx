import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Checkbox, Divider, Group, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import { FieldRow, InspectorButton, InspectorCard, InspectorTextInput } from "../../components/inspector";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { isResoniteUrl } from "../../lib/resoniteUrl";

// スポーン / インパルス（R14・フォーカス中セッションへ）。
//   アイテムスポーン       = spawn "<url>" <active> <persistent>
//   ダイナミックインパルス = dynamicimpulsestring "<tag>" "<value>"（tag 必須・value 任意）
// どちらも非破壊操作なので確認ダイアログなし＝実行→受理トースト（方針A・respawn/message と同格）。
// spawn/impulse はセッションの users/status を変えない（再取得不要）ため onChanged は受け取らない。
export function SpawnImpulseCard({ idx }: { idx: number }) {
  const { t } = useTranslation();
  const { busy, run } = useAsyncAction();
  const [url, setUrl] = useState("");
  const [active, setActive] = useState(true);
  const [persistent, setPersistent] = useState(false);
  const [tag, setTag] = useState("");
  const [value, setValue] = useState("");

  const urlValid = isResoniteUrl(url);
  const tagValid = tag.trim() !== "";

  return (
    <InspectorCard title={t("session.spawnImpulse")}>
      <Stack gap={10}>
        {/* アイテムスポーン */}
        <Text size="xs" fw={700} c="dimmed">
          {t("session.spawnSection")}
        </Text>
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
            disabled={busy || !urlValid}
            onClick={() =>
              void run(() => api.spawnItem(idx, url.trim(), active, persistent), t("toast.spawnDone"))
            }
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
      </Stack>
    </InspectorCard>
  );
}
