import { useEffect } from "react";
import { Group, Stack, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import type { Status } from "../../api";
import { InspectorCard } from "../../components/inspector";
import { AccountForm } from "./AccountForm";
import { SaveButton } from "./SaveButton";
import { useCredentialsForm } from "./useCredentialsForm";

// 稼働中ヘッドレスの Resonite ログイン状態を1行で表示する（軽量版・残課題の表面化）。
// loginState は起動ログから検出（headless 側）。稼働中のみ意味があるため running 時だけ出す。
function LoginStatusLine({ status }: { status: Status | null }) {
  const { t } = useTranslation();
  if (!status || status.state !== "running" || !status.loginState) return null;

  let color = "dimmed";
  let body = t("settings.loginAnonymous");
  if (status.loginState === "loggedIn") {
    color = "teal.4";
    body = `${t("settings.loginLoggedIn")}（${status.loginUserId ?? ""}）`;
  } else if (status.loginState === "failed") {
    color = "red.5";
    body = t("settings.loginFailed");
  }

  return (
    <Group gap={6} wrap="nowrap">
      <Text size="xs" c="dimmed">
        {t("settings.loginStateLabel")}:
      </Text>
      <Text size="xs" c={color}>
        {body}
      </Text>
    </Group>
  );
}

// 中央 Resonite アカウント（設定タブ）。初回モーダルと同じ useCredentialsForm/AccountForm を共用。
export function AccountSection({ onSaved, status }: { onSaved?: () => void; status?: Status | null }) {
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
        <LoginStatusLine status={status ?? null} />
        <SaveButton label={t("settings.save")} onClick={f.save} disabled={!f.canSave} loading={f.busy} />
      </Stack>
    </InspectorCard>
  );
}
