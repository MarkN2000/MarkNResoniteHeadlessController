import { useTranslation } from "react-i18next";
import { Button, Center, Stack, Text, Title } from "@mantine/core";
import type { UpdateInfo } from "../api";

// MRHC 終了依頼後の静止画面（docs/design/self-update.md）。サーバーは停止済みで以後の
// API は全て失敗するため、通常 UI を丸ごと置き換えて再起動の案内だけを表示する。
// info は終了直前の更新チェック結果をそのまま受け取る（goos=手順の出し分け・staged=次回の版）。
export function ShutdownScreen({ info }: { info: UpdateInfo }) {
  const { t } = useTranslation();
  return (
    <Center h="100%">
      <Stack align="center" gap="sm" p="md" style={{ maxWidth: 480 }}>
        <Title order={3}>{t("shutdown.title")}</Title>
        <Text ta="center">{t(info.goos === "windows" ? "shutdown.bodyWindows" : "shutdown.bodyLinux")}</Text>
        {info.staged && (
          <Text size="sm" c="dimmed">
            {t("shutdown.stagedNote", { version: info.staged })}
          </Text>
        )}
        <Button mt="sm" onClick={() => window.location.reload()}>
          {t("shutdown.reload")}
        </Button>
      </Stack>
    </Center>
  );
}
