import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Anchor, Button, Group, List, Loader, Modal, Stack, Text } from "@mantine/core";
import * as api from "../api";
import type { UpdateInfo } from "../api";

// リリースノートへのリンク（公開ページ。バックエンドの取得元と同じリポジトリ）。
const RELEASES_URL = "https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases";

// 自己更新モーダル（docs/design/self-update.md）。表示は info から導出する:
//   staged あり        → 適用済み（再起動手順＋「今すぐ終了」）
//   updateAvailable    → 適用前（アップデートボタン）
//   currentIsRelease外 → 開発ビルド（適用不可）／それ以外 → 最新です
// 開くたびに再チェックする（ログイン時のバッジ情報が古い可能性があるため）。
export function UpdateModal({
  opened,
  onClose,
  info,
  onInfoChange,
  onShutdownDone,
}: {
  opened: boolean;
  onClose: () => void;
  info: UpdateInfo | null;
  onInfoChange: (i: UpdateInfo | null) => void;
  onShutdownDone: (info: UpdateInfo) => void;
}) {
  const { t } = useTranslation();
  const [checking, setChecking] = useState(false);
  const [applying, setApplying] = useState(false);
  const [shuttingDown, setShuttingDown] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // 最新の info を deps に入れず参照するための ref（deps に入れると onInfoChange→info 変化→
  // recheck 再生成→useEffect 再発火のチェックループになる）。
  const infoRef = useRef(info);
  infoRef.current = info;

  const recheck = useCallback(() => {
    setChecking(true);
    setError(null);
    void api.checkUpdate().then((i) => {
      // MRHC 自体に不達（null）のときは保持済みの staged 表示を消さない
      //（staged はローカル状態で、応答が無くても正しさが変わらないため）。
      if (i || !infoRef.current?.staged) onInfoChange(i);
      setChecking(false);
    });
  }, [onInfoChange]);
  useEffect(() => {
    if (opened) recheck();
  }, [opened, recheck]);

  // 適用エラーの code → locale 変換（モーダル内に赤文字で表示。未知 code は生メッセージ）。
  function applyErrorText(code?: string, raw?: string): string {
    switch (code) {
      case "update_busy":
        return t("update.errBusy");
      case "up_to_date":
        return t("update.errUpToDate");
      case "no_release":
        return t("update.errNoRelease");
      case "not_release_build":
        return t("update.errNotReleaseBuild");
      case "exe_dir_not_writable":
        return t("update.errNotWritable");
      case "network":
        return t("update.errNetwork");
      default:
        // 未知 code（update_failed 等）は steam と同じく「locale 見出し + 生 detail」の併記
        //（生 detail は日本語のため、見出し無しだと英語 UI に日本語だけが出てしまう）。
        return raw ? `${t("update.errFailed")}: ${raw}` : t("update.errFailed");
    }
  }

  async function apply() {
    setApplying(true);
    setError(null);
    const r = await api.applyUpdate();
    setApplying(false);
    if (r.ok && r.staged) {
      // 以後の表示（バッジ・メニュー・本モーダル）を「再起動待ち」へ切り替える。
      onInfoChange(info ? { ...info, staged: r.staged } : info);
    } else {
      setError(applyErrorText(r.code, r.error));
    }
  }

  async function shutdownNow() {
    setShuttingDown(true);
    setError(null);
    const r = await api.shutdownApp();
    // info はボタンが staged 分岐（info 非 null）でしか描画されないため成功時は常にある。
    if (r.ok && info) {
      onShutdownDone(info); // App 全体を終了後の静止画面へ（サーバーは停止する＝以後の API は失敗）
      return;
    }
    setShuttingDown(false);
    setError(t("update.shutdownFailed"));
  }

  const staged = info?.staged;
  const win = info?.goos === "windows"; // 使うのは staged 分岐内＝info 非 null 時のみ
  const title = staged ? t("update.pendingTitle", { version: staged }) : t("update.title");

  let body: React.ReactNode;
  if (checking) {
    body = (
      <Group gap="xs">
        <Loader size="sm" />
        <Text size="sm" c="dimmed">
          {t("update.checking")}
        </Text>
      </Group>
    );
  } else if (!info) {
    // チェック失敗（GitHub 不達・オフライン等）。
    body = (
      <Stack gap="sm">
        <Text size="sm">{t("update.checkFailed")}</Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t("update.close")}
          </Button>
          <Button onClick={recheck}>{t("update.retry")}</Button>
        </Group>
      </Stack>
    );
  } else if (staged) {
    // 適用済み・再起動待ち: 「今すぐ終了」と手動再起動の手順を1画面で示す。
    body = (
      <Stack gap="sm">
        <Text fw={600}>✅ {t("update.applied", { version: staged })}</Text>
        <Text size="sm">{t("update.appliedBody", { current: info.current, version: staged })}</Text>
        {error && (
          <Text size="sm" c="red">
            {error}
          </Text>
        )}
        <Button color="red" autoContrast={false} loading={shuttingDown} onClick={() => void shutdownNow()}>
          {t("update.shutdownNow")}
        </Button>
        <Text size="xs" c="dimmed">
          {t("update.shutdownNowNote")}
        </Text>
        <Text size="sm" fw={600} mt="xs">
          {t("update.shutdownLater")}
        </Text>
        <List size="sm" type="ordered" spacing="xs">
          <List.Item>
            {t(win ? "update.step1Windows" : "update.step1Linux")}
            <br />
            <Text span size="xs" c="orange">
              {t(win ? "update.step1WarnWindows" : "update.step1WarnLinux")}
            </Text>
          </List.Item>
          <List.Item>{t(win ? "update.step2Windows" : "update.step2Linux")}</List.Item>
          <List.Item>{t("update.step3")}</List.Item>
        </List>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t("update.close")}
          </Button>
        </Group>
      </Stack>
    );
  } else if (info.checkFailed) {
    // GitHub への確認失敗（staged は上の分岐が先に拾う＝ここはローカルに見せるものが無いケース）。
    body = (
      <Stack gap="sm">
        <Text size="sm">
          {info.checkError === "no_release" ? t("update.errNoRelease") : t("update.checkFailed")}
        </Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t("update.close")}
          </Button>
          <Button onClick={recheck}>{t("update.retry")}</Button>
        </Group>
      </Stack>
    );
  } else if (!info.currentIsRelease) {
    body = (
      <Stack gap="sm">
        <Text size="sm" c="dimmed">
          {t("update.devBuild", { version: info.current })}
        </Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t("update.close")}
          </Button>
        </Group>
      </Stack>
    );
  } else if (info.updateAvailable) {
    body = (
      <Stack gap="sm">
        <Text>{t("update.available", { latest: info.latest, current: info.current })}</Text>
        <Anchor href={RELEASES_URL} target="_blank" rel="noreferrer" size="sm">
          {t("update.releaseNotes")}
        </Anchor>
        {error && (
          <Text size="sm" c="red">
            {error}
          </Text>
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t("update.close")}
          </Button>
          <Button loading={applying} onClick={() => void apply()}>
            {t("update.apply")}
          </Button>
        </Group>
      </Stack>
    );
  } else {
    body = (
      <Stack gap="sm">
        <Text size="sm">{t("update.upToDate", { version: info.current })}</Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t("update.close")}
          </Button>
        </Group>
      </Stack>
    );
  }

  return (
    // 適用中・終了依頼中は閉じさせない（押した操作の結果を見届けさせる）。
    // Esc（closeOnEscape は Mantine 既定 true）も同条件で無効化する。
    <Modal
      opened={opened}
      onClose={onClose}
      title={title}
      centered
      closeOnClickOutside={!applying && !shuttingDown}
      closeOnEscape={!applying && !shuttingDown}
      withCloseButton={!applying && !shuttingDown}
    >
      {body}
    </Modal>
  );
}
