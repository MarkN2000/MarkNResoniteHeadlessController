import { Box, ScrollArea, Stack } from "@mantine/core";
import type { Status } from "../../api";
import { SplitColumns } from "../../components/SplitColumns";
import { PasswordSection } from "./PasswordSection";
import { AccountSection } from "./AccountSection";
import { AppSettingsSection } from "./AppSettingsSection";
import { SteamSection } from "./SteamSection";
import { CacheSection } from "./CacheSection";
import { QUICSection } from "./QUICSection";

// 設定タブ（7-5・§3.15）。停止中でも使える（アプリ/ファイル設定系）。
// レイアウト: 他タブと同じ SplitColumns（xl 以上で左右2カラム＝横幅を活用・未満は1カラム中央寄せ）。
//   左 = パスワード / アカウント / アプリ設定、右 = Steam DD設定 / キャッシュ管理。
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
      <Box pb="md">
        <SplitColumns
          left={
            <Stack gap="md">
              <PasswordSection />
              <AccountSection onSaved={onCredentialsChanged} status={status} />
              <AppSettingsSection />
            </Stack>
          }
          right={
            <Stack gap="md">
              <SteamSection status={status} />
              <QUICSection />
              <CacheSection status={status} />
            </Stack>
          }
        />
      </Box>
    </ScrollArea>
  );
}
