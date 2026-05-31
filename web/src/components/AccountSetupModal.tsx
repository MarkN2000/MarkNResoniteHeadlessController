import { useEffect } from "react";
import { Button, Group, Modal, Stack, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { AccountForm } from "../tabs/settings/AccountForm";
import { useCredentialsForm } from "../tabs/settings/useCredentialsForm";

// 初回オンボーディング: Resonite アカウント未設定のとき、ログイン直後に1回だけ出すモーダル。
// 設定タブの AccountSection と同じ useCredentialsForm/AccountForm を共用（UI 共通化・案A）。
// 「後で」で閉じられる（強制ブロックはしない）。閉じても未設定バナーが鳴り続ける（App 側）。
// 初回設定なので password も必須（canSave が pw 未登録時は password を要求する）。
export function AccountSetupModal({
  opened,
  onClose,
  onSaved,
}: {
  opened: boolean;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const f = useCredentialsForm(() => {
    onSaved();
    onClose();
  });
  // 開いたとき既存値（あれば username）を読み込む。
  useEffect(() => {
    if (opened) void f.load();
  }, [opened, f.load]);

  return (
    <Modal opened={opened} onClose={onClose} title={t("settings.setupTitle")} centered>
      <Stack gap="sm">
        <Text size="sm">{t("settings.setupIntro")}</Text>
        <AccountForm
          username={f.username}
          password={f.password}
          onUsername={f.setUsername}
          onPassword={f.setPassword}
          passwordPlaceholder={t("settings.password")}
        />
        <Group justify="flex-end" gap="xs" mt="xs">
          <Button variant="default" onClick={onClose}>
            {t("settings.setupLater")}
          </Button>
          <Button
            color="brand"
            variant="filled"
            styles={{ label: { color: "var(--mantine-color-dark-9)" } }}
            disabled={!f.canSave}
            loading={f.busy}
            onClick={f.save}
          >
            {t("settings.save")}
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
