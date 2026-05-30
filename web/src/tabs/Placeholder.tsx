import { useTranslation } from "react-i18next";
import { Center, Stack, Text, Title } from "@mantine/core";

// 後続段階で実装するタブの仮表示。
export function TabPlaceholder({ titleKey }: { titleKey: string }) {
  const { t } = useTranslation();
  return (
    <Center h="100%">
      <Stack align="center" gap="xs">
        <Title order={3}>{t(titleKey)}</Title>
        <Text c="dimmed" size="sm">
          {t("placeholder.comingSoon")}
        </Text>
      </Stack>
    </Center>
  );
}

// 停止中・ヘッドレス稼働が要るタブで表示する誘導。
export function StartPrompt() {
  const { t } = useTranslation();
  return (
    <Center h="100%">
      <Text c="dimmed">{t("placeholder.startFirst")}</Text>
    </Center>
  );
}
