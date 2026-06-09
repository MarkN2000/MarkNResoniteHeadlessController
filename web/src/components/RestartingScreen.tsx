import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Button, Center, Loader, Stack, Text, Title } from "@mantine/core";
import * as api from "../api";
import type { UpdateInfo } from "../api";

// 自己更新の「今すぐ再起動」後の画面（docs/design/self-update.md）。サーバーは graceful 停止→
// 新バイナリ起動の最中で、通常 UI（/api/v1/events の SSE 等）を丸ごと置き換える。サーバー復帰を
// ポーリングし、応答が返ったら自動でリロードして新プロセスの UI へ戻る（停止が長いと数分かかりうる）。
// info は再起動直前の更新チェック結果（staged=再起動後の版）。
export function RestartingScreen({ info }: { info: UpdateInfo }) {
  const { t } = useTranslation();
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      if (stopped) return;
      if (await api.pingAlive()) {
        window.location.reload(); // 復帰（セッション切れなら再読込先で Login へ）
        return;
      }
      if (!stopped) timer = setTimeout(tick, 3000);
    };
    // 即時には叩かず、停止が始まる猶予を置いてからポーリング開始。
    timer = setTimeout(tick, 3000);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, []);

  return (
    <Center h="100%">
      <Stack align="center" gap="sm" p="md" style={{ maxWidth: 480 }}>
        <Loader />
        <Title order={3}>{t("restarting.title")}</Title>
        <Text ta="center">{t("restarting.body")}</Text>
        {info.staged && (
          <Text size="sm" c="dimmed">
            {t("restarting.stagedNote", { version: info.staged })}
          </Text>
        )}
        <Text size="xs" c="dimmed" ta="center">
          {t("restarting.manualHint")}
        </Text>
        <Button mt="sm" variant="default" onClick={() => window.location.reload()}>
          {t("restarting.reload")}
        </Button>
      </Stack>
    </Center>
  );
}
