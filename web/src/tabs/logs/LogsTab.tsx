import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Center, Group, Loader, Stack, Text, Textarea } from "@mantine/core";
import * as api from "../../api";
import type { LogContent, LogFileInfo } from "../../api";
import { FieldRow, InspectorButton, InspectorCard, InspectorSelect, RefreshButton } from "../../components/inspector";
import { copyText } from "../../lib/clipboard";
import { formatBytes } from "../../lib/format";
import { notifyError, notifyInfo } from "../../lib/notify";

// Resonite ログ閲覧タブ（読み取り専用）。{InstallDir}/Headless/Logs のログを
// ドロップダウンで選び、本文を等幅表示・コピー・全文ダウンロードできる。稼働中/停止中どちらでも閲覧可。
// 表示とコピーは末尾10MiBのみ取得（backend で切り詰め）、ダウンロードは元ファイルを直接取得する。
export function LogsTab() {
  const { t } = useTranslation();
  const [files, setFiles] = useState<LogFileInfo[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [content, setContent] = useState<LogContent | null>(null);
  const [listLoading, setListLoading] = useState(false);
  const [contentLoading, setContentLoading] = useState(false);
  const [contentError, setContentError] = useState(false);

  const loadList = useCallback(async () => {
    setListLoading(true);
    const fs = await api.getLogFiles();
    setListLoading(false);
    setFiles(fs);
    // 選択が無い/消えていたら先頭（最新ログ）を選ぶ。
    setSelected((cur) => (cur && fs.some((f) => f.name === cur) ? cur : (fs[0]?.name ?? null)));
  }, []);
  useEffect(() => {
    void loadList();
  }, [loadList]);

  const loadContent = useCallback(async (name: string) => {
    setContentLoading(true);
    setContentError(false);
    const c = await api.getLogContent(name);
    setContentLoading(false);
    setContent(c);
    // null=取得失敗（稼働中の現行ログがロックされている等）。空表示で黙らせず明示する。
    setContentError(c === null);
  }, []);
  useEffect(() => {
    if (selected) void loadContent(selected);
    else setContent(null);
  }, [selected, loadContent]);

  // 更新: 一覧と（選択中があれば）本文を再取得。ログは伸び続けるため手動更新を提供。
  const onRefresh = () => {
    void loadList();
    if (selected) void loadContent(selected);
  };

  const onCopy = async () => {
    if (!content) return;
    const ok = await copyText(content.content);
    if (ok) notifyInfo(t("logs.copied"));
    else notifyError(t("logs.copyFailed"));
  };

  const onDownload = () => {
    if (!selected) return;
    // 同一オリジンの認証Cookieを使ってブラウザへ直接保存させ、ログ全文をJSメモリへ載せない。
    const link = document.createElement("a");
    link.href = api.getLogDownloadUrl(selected);
    link.download = selected;
    document.body.appendChild(link);
    link.click();
    link.remove();
  };

  const sel = files.find((f) => f.name === selected);
  const options = files.map((f) => ({ value: f.name, label: f.name }));

  return (
    <Stack h="100%" gap="sm" p="md">
      <InspectorCard
        title={t("logs.title")}
        actions={<RefreshButton onClick={onRefresh} loading={listLoading} label={t("logs.refresh")} />}
      >
        <Stack gap={6}>
          <FieldRow label={t("logs.file")}>
            <InspectorSelect
              data={options}
              value={selected}
              onChange={setSelected}
              placeholder={t("logs.noFiles")}
              disabled={files.length === 0}
            />
          </FieldRow>
          {sel && (
            <Text size="xs" c="dimmed" ta="right">
              {formatBytes(sel.size)} ・ {new Date(sel.modTime).toLocaleString()}
            </Text>
          )}
          {content?.truncated && (
            <Text size="xs" c="yellow.6">
              {t("logs.truncated")}
            </Text>
          )}
          <Group justify="flex-end">
            <InspectorButton onClick={onCopy} disabled={!content || contentLoading}>
              {t("logs.copy")}
            </InspectorButton>
            <InspectorButton onClick={onDownload} disabled={!content || contentLoading}>
              {t("logs.download")}
            </InspectorButton>
          </Group>
        </Stack>
      </InspectorCard>

      <Box style={{ flex: 1, minHeight: 0 }}>
        {contentLoading ? (
          <Center h="100%">
            <Loader size="sm" />
          </Center>
        ) : files.length === 0 ? (
          <Center h="100%">
            <Text c="dimmed" size="sm">
              {t("logs.empty")}
            </Text>
          </Center>
        ) : contentError ? (
          <Center h="100%">
            <Text c="red.5" size="sm">
              {t("logs.loadError")}
            </Text>
          </Center>
        ) : (
          // 大量テキストに強い読み取り専用 textarea（横は折り返さず内部スクロール）。
          <Textarea
            readOnly
            value={content?.content ?? ""}
            styles={{
              root: { height: "100%" },
              wrapper: { height: "100%" },
              input: {
                height: "100%",
                fontFamily: "monospace",
                fontSize: 12,
                lineHeight: 1.5,
                whiteSpace: "pre",
                overflow: "auto",
                resize: "none",
              },
            }}
          />
        )}
      </Box>
    </Stack>
  );
}
