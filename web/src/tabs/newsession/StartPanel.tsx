import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Group, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { WorldResult } from "../../api";
import {
  FieldRow,
  InspectorButton,
  InspectorCard,
  InspectorSelect,
  InspectorTextInput,
} from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import { isResoniteUrl, parseResrecUrl } from "../../lib/resoniteUrl";
import { StarButton } from "../../components/worldsearch/StarButton";

// 新規セッションの起動方法（URL / テンプレート）。起動は確認 → 実行 → onStarted（一覧再取得）。
// 結果トーストは useConfirm（onConfirm が WriteResult を返す）で自動（7-7 第1層）。
// お気に入り（isFavorited/onToggleFavorite）は親 NewSessionTab から受領（単一の真実源）。
export function StartPanel({
  onStarted,
  isFavorited,
  onToggleFavorite,
}: {
  onStarted: () => void;
  isFavorited: (recordId: string) => boolean;
  onToggleFavorite: (wld: WorldResult) => void;
}) {
  const { t } = useTranslation();
  const [template, setTemplate] = useState<string>(api.WORLD_TEMPLATES[0]);
  const [url, setUrl] = useState("");
  const confirm = useConfirm();

  const urlValid = isResoniteUrl(url);

  // お気に入り登録可能なのは resrec:///U|G-xxx/R-xxx 厳密形式のみ（他スキームは null＝★無効）。
  const resrec = parseResrecUrl(url);
  const favorited = resrec ? isFavorited(resrec.recordId) : false;

  // URL からは name/thumbnailUrl を取得できないため空で保存（方針: 名前なしで保存）。
  // resoniteUrl はパース結果から正規化生成し backend の検証に確実に一致させる。
  const toggleFavorite = () => {
    if (!resrec) return;
    onToggleFavorite({
      name: "",
      ownerId: resrec.ownerId,
      recordId: resrec.recordId,
      resoniteUrl: `resrec:///${resrec.ownerId}/${resrec.recordId}`,
      thumbnailUrl: "",
    });
  };

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
              <StarButton
                active={favorited}
                disabled={!resrec}
                onClick={toggleFavorite}
                label={
                  resrec
                    ? favorited
                      ? t("newSession.removeFavorite")
                      : t("newSession.addFavorite")
                    : t("newSession.favoriteUrlOnly")
                }
              />
            </Group>
            {url.trim() !== "" && !urlValid && (
              <Text size="xs" c="dimmed">
                {t("newSession.urlHint")}
              </Text>
            )}
          </Stack>
        </FieldRow>
      </Stack>

      <ConfirmHost confirm={confirm} />
    </InspectorCard>
  );
}
