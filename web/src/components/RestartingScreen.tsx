import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Button, Center, Loader, Stack, Text, Title } from "@mantine/core";
import * as api from "../api";
import type { UpdateInfo } from "../api";

// 自己更新の「今すぐ再起動」後の画面（docs/design/self-update.md）。サーバーは graceful 停止→
// 新バイナリ起動の最中で、通常 UI（/api/v1/events の SSE 等）を丸ごと置き換える。サーバー復帰を
// ポーリングし、新プロセスを検出したら自動でリロードして新版の UI へ戻る（停止が長いと数分かかりうる）。
// info は再起動直前の更新チェック結果（staged=再起動後の版）。oldBoot は再起動直前に捕捉した旧
// プロセスの boot 識別子（新プロセス検出の基準。取得失敗時 null）。
export function RestartingScreen({ info, oldBoot }: { info: UpdateInfo; oldBoot: string | null }) {
  const { t } = useTranslation();
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    // 重要: 再起動要求後も旧 HTTP サーバーはヘッドレス停止中（最大 ~185s）応答し続ける。
    // そのため「応答あり＝復帰」では旧サーバーへ誤って戻ってしまう。プロセス毎の boot 識別子
    //（無認証 /ping）が oldBoot から変化した時点を「新プロセスが起動した」とみなして reload する
    // （タイミングに依存せず確実）。oldBoot が取れていなければ最初の観測値を基準にフォールバック。
    let base = oldBoot;
    const tick = async () => {
      if (stopped) return;
      const boot = await api.fetchBootID();
      if (boot !== null) {
        if (base === null) {
          base = boot; // フォールバック: 最初の観測を基準にする
        } else if (boot !== base) {
          window.location.reload(); // boot が変化＝新プロセス（セッション切れなら再読込先で Login へ）
          return;
        }
      }
      if (!stopped) timer = setTimeout(tick, 1500);
    };
    timer = setTimeout(tick, 1500);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [oldBoot]);

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
