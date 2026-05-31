import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Group, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import {
  FieldRow,
  InspectorButton,
  InspectorCard,
  InspectorSelect,
  InspectorTextInput,
} from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useConfirm } from "../../hooks/useConfirm";

// URL の scheme 検証（v1 踏襲）: res:// / resrec:// / res-steam:// などで始まること。
// 方針A 上、不正 URL でも backend は HTTP 200 を返し得る（＝無音失敗）ため、空振りをここで減らす。
const URL_SCHEME = /^res[-\w]*:\/\//i;

// 新規セッションの起動方法（URL / テンプレート）。起動は確認 → 実行 → onStarted（一覧再取得）。
// 結果トーストは useConfirm（onConfirm が WriteResult を返す）で自動（7-7 第1層）。
export function StartPanel({ onStarted }: { onStarted: () => void }) {
  const { t } = useTranslation();
  const [template, setTemplate] = useState<string>(api.WORLD_TEMPLATES[0]);
  const [url, setUrl] = useState("");
  const confirm = useConfirm();

  const urlValid = URL_SCHEME.test(url.trim());

  // 起動の確認 → 実行 → onStarted。op は WriteResult を返すラッパ。confirm.busy が
  // ConfirmModal の loading を駆動（startworldurl は最大60s かかり得る）。
  const askStart = (message: string, op: () => Promise<unknown>) =>
    confirm.ask({
      title: t("newSession.confirmTitle"),
      message,
      success: t("toast.newSessionDone"),
      onConfirm: async () => {
        const r = await op();
        onStarted();
        return r;
      },
    });

  return (
    <InspectorCard title={t("newSession.launchTitle")}>
      <Stack gap={10}>
        <FieldRow label={t("newSession.template")}>
          <Group gap="xs" wrap="nowrap">
            <InspectorSelect
              data={[...api.WORLD_TEMPLATES]}
              value={template}
              onChange={(v) => v && setTemplate(v)}
              style={{ flex: 1, minWidth: 0 }}
            />
            <InspectorButton
              onClick={() =>
                askStart(t("newSession.confirmTemplate", { template }), () => api.startWorldTemplate(template))
              }
            >
              {t("newSession.start")}
            </InspectorButton>
          </Group>
        </FieldRow>

        <FieldRow label={t("newSession.url")} align="start">
          <Stack gap={4}>
            <Group gap="xs" wrap="nowrap">
              <InspectorTextInput
                value={url}
                onChange={(e) => setUrl(e.currentTarget.value)}
                placeholder={t("newSession.urlPlaceholder")}
                style={{ flex: 1, minWidth: 0 }}
              />
              <InspectorButton
                disabled={!urlValid}
                onClick={() => askStart(t("newSession.confirmUrl"), () => api.startWorldURL(url.trim()))}
              >
                {t("newSession.start")}
              </InspectorButton>
            </Group>
            {url.trim() !== "" && !urlValid && (
              <Text size="xs" c="dimmed">
                {t("newSession.urlHint")}
              </Text>
            )}
          </Stack>
        </FieldRow>
      </Stack>

      <ConfirmModal
        opened={confirm.request !== null}
        title={confirm.request?.title ?? ""}
        message={confirm.request?.message}
        loading={confirm.busy}
        onConfirm={() => void confirm.confirm()}
        onClose={confirm.close}
      />
    </InspectorCard>
  );
}
