import { useEffect } from "react";
import { Stack, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { InspectorCard } from "../../components/inspector";
import { AccountForm } from "./AccountForm";
import { SaveButton } from "./SaveButton";
import { useCredentialsForm } from "./useCredentialsForm";

// 中央 Resonite アカウント（設定タブ）。初回モーダルと同じ useCredentialsForm/AccountForm を共用。
export function AccountSection({ onSaved }: { onSaved?: () => void }) {
  const { t } = useTranslation();
  const f = useCredentialsForm(onSaved);
  useEffect(() => {
    void f.load();
  }, [f.load]);

  return (
    <InspectorCard title={t("settings.accountSection")}>
      <Stack gap={8}>
        <Text size="xs" c="dimmed">
          {t("settings.accountDesc")}
        </Text>
        <AccountForm
          username={f.username}
          password={f.password}
          onUsername={f.setUsername}
          onPassword={f.setPassword}
          passwordPlaceholder={f.hasPassword ? t("settings.passwordKeep") : undefined}
        />
        <SaveButton label={t("settings.save")} onClick={f.save} disabled={!f.canSave} loading={f.busy} />
      </Stack>
    </InspectorCard>
  );
}
