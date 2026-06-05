import { Box, ScrollArea, Stack } from "@mantine/core";
import type { Status } from "../../api";
import { PasswordSection } from "./PasswordSection";
import { AccountSection } from "./AccountSection";
import { AppSettingsSection } from "./AppSettingsSection";
import { SteamSection } from "./SteamSection";

// 設定タブ（7-5・§3.15）。停止中でも使える（アプリ/ファイル設定系）。
// 単一中央カラム（session タブ単一カラム時と同じ最大560・中央寄せ・ページ全体スクロール）。
// onCredentialsChanged: アカウント保存後に App の未設定バナー/初回モーダル状態を再評価させる。
// status: 稼働中ヘッドレスの Resonite ログイン状態をアカウント欄に表示するため。
export function SettingsTab({
  onCredentialsChanged,
  status,
}: {
  onCredentialsChanged: () => void;
  status: Status | null;
}) {
  return (
    <ScrollArea h="100%" type="hover">
      <Box pb="md" mx="auto" maw={560}>
        <Stack gap="md">
          <PasswordSection />
          <AccountSection onSaved={onCredentialsChanged} status={status} />
          <AppSettingsSection />
          <SteamSection status={status} />
        </Stack>
      </Box>
    </ScrollArea>
  );
}
