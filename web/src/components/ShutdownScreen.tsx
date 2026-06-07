import { useTranslation } from "react-i18next";
import { Button, Center, Stack, Text, Title } from "@mantine/core";

// MRHC 終了依頼後の静止画面（docs/design/self-update.md）。サーバーは停止済みで以後の
// API は全て失敗するため、通常 UI を丸ごと置き換えて再起動の案内だけを表示する。
export function ShutdownScreen({ goos, staged }: { goos: string; staged?: string }) {
  const { t } = useTranslation();
  return (
    <Center h="100%">
      <Stack align="center" gap="sm" p="md" style={{ maxWidth: 480 }}>
        <Title order={3}>{t("shutdown.title")}</Title>
        <Text ta="center">{t(goos === "windows" ? "shutdown.bodyWindows" : "shutdown.bodyLinux")}</Text>
        {staged && (
          <Text size="sm" c="dimmed">
            {t("shutdown.stagedNote", { version: staged })}
          </Text>
        )}
        <Button mt="sm" onClick={() => window.location.reload()}>
          {t("shutdown.reload")}
        </Button>
      </Stack>
    </Center>
  );
}
