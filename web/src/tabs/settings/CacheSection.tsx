import { useCallback, useEffect, useState } from "react";
import { Center, Divider, Group, Loader, Stack, Switch, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import type { CacheConfig, Status } from "../../api";
import { FieldRow, InspectorButton, InspectorCard, InspectorNumberInput } from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useConfirm } from "../../hooks/useConfirm";
import { formatBytes } from "../../lib/format";
import { SaveButton } from "./SaveButton";

// キャッシュ管理（既定 {dataDir}/headless-cache）。
//   - 停止時の自動「古いファイル削除」（トグル＋日数・既定 OFF/30日）。
//   - パス表示＋「サイズを計算」（走査するためボタン押下時のみ）。
//   - 全キャッシュ削除（停止中のみ・確認ダイアログ）。
// status は全削除ボタンの停止中ガード表示に使う。
export function CacheSection({ status }: { status: Status | null }) {
  const { t } = useTranslation();
  const [orig, setOrig] = useState<CacheConfig | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [days, setDays] = useState<number | string>(30);
  const [path, setPath] = useState("");
  const [loadFailed, setLoadFailed] = useState(false);
  const [size, setSize] = useState<number | null>(null); // null=未計算
  const sizeAction = useAsyncAction();
  const save = useAsyncAction();
  const confirm = useConfirm();

  // 設定＋パスを取得（パスは getConfigDefaults の cacheFolder＝walk 不要。サイズはボタンで別途）。
  const load = useCallback(async () => {
    const [c, defaults] = await Promise.all([api.getCacheConfig(), api.getConfigDefaults()]);
    if (c) {
      setOrig(c);
      setEnabled(c.enabled);
      setDays(c.maxAgeDays);
      setLoadFailed(false);
    } else {
      setLoadFailed(true);
    }
    if (defaults) setPath(defaults.cacheFolder);
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  const daysNum = Number(days);
  const daysValid = Number.isInteger(daysNum) && daysNum >= 1;
  const dirty = !!orig && (enabled !== orig.enabled || (daysValid && daysNum !== orig.maxAgeDays));
  const canSave = daysValid && dirty;

  const onSave = () =>
    save.run(async () => {
      const body: CacheConfig = { enabled, maxAgeDays: daysNum };
      const r = await api.putCacheConfig(body);
      if (r.ok) setOrig(body);
      return r;
    }, t("settings.toastCacheSaved"));

  // サイズ集計は走査するためボタン押下時のみ（巨大キャッシュでも他操作は止めない）。
  const onComputeSize = () =>
    sizeAction.run(async () => {
      const info = await api.getCacheInfo();
      if (info) {
        setSize(info.sizeBytes);
        if (info.path) setPath(info.path);
      }
    });

  const stopped = status?.state === "stopped";
  const onClear = () =>
    confirm.ask({
      title: t("settings.cacheClearConfirmTitle"),
      message: t("settings.cacheClearConfirmMsg"),
      danger: true,
      success: t("settings.toastCacheCleared"),
      onConfirm: async () => {
        const r = await api.clearCache();
        if (r.ok) setSize(0); // 全削除後は 0
        return r;
      },
    });

  return (
    <InspectorCard title={t("settings.cacheSection")}>
      {!orig ? (
        <Center h={60}>
          {loadFailed ? (
            <Text size="sm" c="red.6">
              {t("settings.loadError")}
            </Text>
          ) : (
            <Loader size="sm" />
          )}
        </Center>
      ) : (
        <Stack gap={8}>
          <Text size="xs" c="dimmed">
            {t("settings.cacheDesc")}
          </Text>

          {/* 停止時の自動「古いファイル削除」 */}
          <FieldRow label={t("settings.cacheAutoEnabled")}>
            <Switch checked={enabled} onChange={(e) => setEnabled(e.currentTarget.checked)} />
          </FieldRow>
          <FieldRow label={t("settings.cacheMaxAgeDays")}>
            <InspectorNumberInput
              value={days}
              onChange={setDays}
              min={1}
              allowNegative={false}
              disabled={!enabled}
            />
          </FieldRow>
          <Text size="xs" c="dimmed">
            {t("settings.cacheAutoNote")}
          </Text>
          <SaveButton label={t("settings.save")} onClick={onSave} disabled={!canSave} loading={save.busy} />

          <Divider my={4} color="dark.4" />

          {/* パス + サイズ + 全削除 */}
          <FieldRow label={t("settings.cachePath")}>
            <Text size="xs" style={{ wordBreak: "break-all" }}>
              {path || "—"}
            </Text>
          </FieldRow>
          <Group justify="space-between" wrap="nowrap" gap="sm">
            <Text size="xs" c="dimmed">
              {size === null ? t("settings.cacheSizeUnknown") : `${t("settings.cacheSize")}: ${formatBytes(size)}`}
            </Text>
            <InspectorButton onClick={onComputeSize} loading={sizeAction.busy} style={{ flexShrink: 0 }}>
              {t("settings.cacheComputeSize")}
            </InspectorButton>
          </Group>

          <InspectorButton severity="danger" fullWidth onClick={onClear} disabled={!stopped}>
            {t("settings.cacheClear")}
          </InspectorButton>
          {!stopped && (
            <Text size="xs" c="dimmed" ta="center">
              {t("settings.cacheClearStoppedHint")}
            </Text>
          )}
        </Stack>
      )}
      <ConfirmHost confirm={confirm} />
    </InspectorCard>
  );
}
