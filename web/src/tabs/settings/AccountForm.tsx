import { Stack } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { FieldRow, InspectorTextInput } from "../../components/inspector";

// 中央 Resonite アカウントの入力フォーム（username + password の2行・presentational）。
// 設定タブ（AccountSection）と初回モーダル（AccountSetupModal）で共用する（UI 共通化・案A）。
// 値と onChange は呼び出し側が持つ。password は config タブと同じ InspectorTextInput type="password"。
export function AccountForm({
  username,
  password,
  onUsername,
  onPassword,
  passwordPlaceholder,
}: {
  username: string;
  password: string;
  onUsername: (v: string) => void;
  onPassword: (v: string) => void;
  passwordPlaceholder?: string;
}) {
  const { t } = useTranslation();
  return (
    <Stack gap={6}>
      <FieldRow label={t("settings.username")}>
        <InspectorTextInput
          value={username}
          onChange={(e) => onUsername(e.currentTarget.value)}
          placeholder={t("settings.usernamePlaceholder")}
        />
      </FieldRow>
      <FieldRow label={t("settings.password")}>
        <InspectorTextInput
          type="password"
          value={password}
          onChange={(e) => onPassword(e.currentTarget.value)}
          placeholder={passwordPlaceholder ?? t("settings.passwordKeep")}
        />
      </FieldRow>
    </Stack>
  );
}
