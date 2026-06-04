import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Center, Group, Image, Loader, SimpleGrid, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { WorldResult } from "../../api";
import { FieldRow, InspectorButton, InspectorCard, InspectorTextInput } from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import { StarButton } from "./StarButton";

// ワールド検索 → 検索結果から起動／お気に入り保存（phase-7-spec §3.12）。
// 検索 = go.resonite.com の公開ワールド（HTML スクレイピング・上位24件）。起動は既存 URL モード
// （startWorldURL に resrec:// を渡す）を流用。お気に入りはサーバー保存（favorites.json）。
// 検索ボタン隣の★トグルでお気に入り表示↔検索結果を切替。各カードの★/☆で登録/解除（無音）。
// お気に入り state（favorites/isFavorited/onToggleFavorite）は親 NewSessionTab から受領（単一の真実源）。
// onStarted = 起動成功後にトップバーのセッション一覧を再取得（StartPanel と同じ）。
export function WorldSearchPanel({
  onStarted,
  favorites,
  isFavorited,
  onToggleFavorite,
}: {
  onStarted: () => void;
  favorites: WorldResult[];
  isFavorited: (recordId: string) => boolean;
  onToggleFavorite: (wld: WorldResult) => void;
}) {
  const { t } = useTranslation();
  const [keyword, setKeyword] = useState("");
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false); // 初回未検索 と 0件 を区別する
  const [searchResults, setSearchResults] = useState<WorldResult[]>([]);
  const [showingFavorites, setShowingFavorites] = useState(false);
  const reqId = useRef(0); // 連打/順序逆転で古い結果を捨てる stale ガード（FriendsTab 流用）
  const confirm = useConfirm();

  const doSearch = async () => {
    const term = keyword.trim();
    if (!term || loading) return;
    const id = ++reqId.current;
    setLoading(true);
    setSearched(true);
    setShowingFavorites(false); // 検索したらお気に入り表示は解除
    const r = await api.searchResoniteWorlds(term);
    if (id !== reqId.current) return; // 古い応答は破棄
    setSearchResults(r);
    setLoading(false);
  };

  // 起動の確認 → startWorldURL → onStarted（一覧再取得）。StartPanel.askStart と同形。
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

  const displayed = showingFavorites ? favorites : searchResults;
  const title = showingFavorites
    ? `${t("newSession.favorites")} (${favorites.length})`
    : searched && !loading
      ? `${t("newSession.searchTitle")} (${searchResults.length})`
      : t("newSession.searchTitle");

  // 空表示（お気に入り0件 / 検索0件）と グリッド表示の出し分け。
  const showEmpty =
    !loading &&
    ((showingFavorites && favorites.length === 0) || (!showingFavorites && searched && searchResults.length === 0));
  const showGrid = !loading && displayed.length > 0;

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
            <StarButton
              active={showingFavorites}
              onClick={() => setShowingFavorites((v) => !v)}
              label={t("newSession.showFavorites")}
            />
          </Group>
        </FieldRow>

        {loading ? (
          <Center py="md">
            <Loader size="sm" />
          </Center>
        ) : showEmpty ? (
          <Text c="dimmed" size="sm" ta="center" py="md">
            {showingFavorites ? t("newSession.noFavorites") : t("newSession.noResults")}
          </Text>
        ) : showGrid ? (
          <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="sm">
            {displayed.map((wld) => (
              <WorldCard
                key={wld.resoniteUrl}
                world={wld}
                favorited={isFavorited(wld.recordId)}
                onStart={() => askStart(wld)}
                onToggleFavorite={() => onToggleFavorite(wld)}
              />
            ))}
          </SimpleGrid>
        ) : null}
      </Stack>

      <ConfirmHost confirm={confirm} />
    </InspectorCard>
  );
}

// 検索結果1件のカード（サムネ＋名前＋所有者ID＋起動＋★）。
// サムネ枠は常に固定高（THUMB_H）のコンテナで確保し、画像はその中を埋める（object-fit: cover）。
// これで「画像の有無・ロード前後」でカード高が変わらず、[起動]/★ ボタンが動かない（押し間違い防止）。
const THUMB_H = 90;

function WorldCard({
  world,
  favorited,
  onStart,
  onToggleFavorite,
}: {
  world: WorldResult;
  favorited: boolean;
  onStart: () => void;
  onToggleFavorite: () => void;
}) {
  const { t } = useTranslation();
  const [imgError, setImgError] = useState(false);
  const showImg = !!world.thumbnailUrl && !imgError; // 空URL or 読込失敗はプレースホルダ
  // 名前なしお気に入り（URL登録）は recordId を代替表示（所有者IDは2行目で識別補助）。
  const displayName = world.name || world.recordId;
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
        <Text fw={600} truncate title={displayName}>
          {displayName}
        </Text>
        <Text size="xs" c="dimmed" truncate>
          {world.ownerId}
        </Text>
      </Box>
      <Group gap="xs" wrap="nowrap">
        <InspectorButton onClick={onStart} style={{ flex: 1, minWidth: 0 }}>
          {t("newSession.start")}
        </InspectorButton>
        <StarButton
          active={favorited}
          onClick={onToggleFavorite}
          label={favorited ? t("newSession.removeFavorite") : t("newSession.addFavorite")}
        />
      </Group>
    </Stack>
  );
}
