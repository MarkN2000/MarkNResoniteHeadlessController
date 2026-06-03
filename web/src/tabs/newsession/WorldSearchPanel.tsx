import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Center, Group, Image, Loader, SimpleGrid, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { WorldResult } from "../../api";
import { FieldRow, InspectorButton, InspectorCard, InspectorTextInput } from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useConfirm } from "../../hooks/useConfirm";

// ワールド検索 → 検索結果から起動する枠（phase-7-spec §3.12）。
// 検索 = go.resonite.com の公開ワールド（HTML スクレイピング・上位24件）。起動は既存 URL モード
// （startWorldURL に resrec:// を渡す）を流用。失敗は getData が [] に吸収＝「該当なし」表示（フレンド検索と同流儀）。
// onStarted = 起動成功後にトップバーのセッション一覧を再取得（StartPanel と同じ）。
export function WorldSearchPanel({ onStarted }: { onStarted: () => void }) {
  const { t } = useTranslation();
  const [keyword, setKeyword] = useState("");
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false); // 初回未検索 と 0件 を区別する
  const [results, setResults] = useState<WorldResult[]>([]);
  const reqId = useRef(0); // 連打/順序逆転で古い結果を捨てる stale ガード（FriendsTab 流用）
  const confirm = useConfirm();

  const doSearch = async () => {
    const term = keyword.trim();
    if (!term || loading) return;
    const id = ++reqId.current;
    setLoading(true);
    setSearched(true);
    const r = await api.searchResoniteWorlds(term);
    if (id !== reqId.current) return; // 古い応答は破棄
    setResults(r);
    setLoading(false);
  };

  // 起動の確認 → startWorldURL → onStarted（一覧再取得）。StartPanel.askStart と同形。
  // confirm.busy が ConfirmModal の loading を駆動（startworldurl は最大60s）。
  const askStart = (wld: WorldResult) =>
    confirm.ask({
      title: t("newSession.confirmTitle"),
      message: t("newSession.confirmWorld", { name: wld.name }),
      success: t("toast.newSessionDone"),
      onConfirm: async () => {
        const r = await api.startWorldURL(wld.resoniteUrl);
        onStarted();
        return r;
      },
    });

  const title =
    searched && !loading
      ? `${t("newSession.searchTitle")} (${results.length})`
      : t("newSession.searchTitle");

  return (
    <InspectorCard title={title}>
      <Stack gap={10}>
        <FieldRow label={t("newSession.keyword")}>
          <Group gap="xs" wrap="nowrap">
            <InspectorTextInput
              value={keyword}
              onChange={(e) => setKeyword(e.currentTarget.value)}
              onKeyDown={(e) => e.key === "Enter" && void doSearch()}
              placeholder={t("newSession.keywordPlaceholder")}
              style={{ flex: 1, minWidth: 0 }}
            />
            <InspectorButton onClick={() => void doSearch()} loading={loading}>
              {t("newSession.search")}
            </InspectorButton>
          </Group>
        </FieldRow>

        {loading ? (
          <Center py="md">
            <Loader size="sm" />
          </Center>
        ) : searched && results.length === 0 ? (
          <Text c="dimmed" size="sm" ta="center" py="md">
            {t("newSession.noResults")}
          </Text>
        ) : results.length > 0 ? (
          <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="sm">
            {results.map((wld) => (
              <WorldCard key={wld.resoniteUrl} world={wld} onStart={() => askStart(wld)} />
            ))}
          </SimpleGrid>
        ) : null}
      </Stack>

      <ConfirmModal
        opened={confirm.request !== null}
        title={confirm.request?.title ?? ""}
        message={confirm.request?.message}
        loading={confirm.busy}
        onConfirm={() => void confirm.confirm()}
        onClose={confirm.close}
      />
    </InspectorCard>
  );
}

// 検索結果1件のカード（サムネ＋名前＋所有者ID＋起動）。
// サムネ枠は常に固定高（THUMB_H）のコンテナで確保し、画像はその中を埋める（object-fit: cover）。
// これで「画像の有無・ロード前後」でカード高が変わらず、[起動]ボタンが動かない（押し間違い防止）。
const THUMB_H = 90;

function WorldCard({ world, onStart }: { world: WorldResult; onStart: () => void }) {
  const { t } = useTranslation();
  const [imgError, setImgError] = useState(false);
  const showImg = !!world.thumbnailUrl && !imgError; // 空URL or 読込失敗はプレースホルダ
  const initial = (world.name || "?").slice(0, 1).toUpperCase();

  return (
    <Stack gap={6}>
      {/* 固定高コンテナ＝レイアウトシフトしない。画像はこの枠を埋め、はみ出しは clip。 */}
      <Box
        h={THUMB_H}
        style={{ backgroundColor: "var(--mantine-color-dark-5)", borderRadius: 8, overflow: "hidden" }}
      >
        {showImg ? (
          <Image
            src={world.thumbnailUrl}
            h="100%"
            w="100%"
            fit="cover"
            alt=""
            onError={() => setImgError(true)} // 読込失敗→頭文字プレースホルダへ（高さは枠で維持）
          />
        ) : (
          <Center h="100%">
            <Text size="lg" c="dimmed">
              {initial}
            </Text>
          </Center>
        )}
      </Box>
      <Box style={{ minWidth: 0 }}>
        <Text fw={600} truncate title={world.name}>
          {world.name}
        </Text>
        <Text size="xs" c="dimmed" truncate>
          {world.ownerId}
        </Text>
      </Box>
      <InspectorButton fullWidth onClick={onStart}>
        {t("newSession.start")}
      </InspectorButton>
    </Stack>
  );
}
