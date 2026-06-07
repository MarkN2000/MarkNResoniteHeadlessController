import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Center, Group, Image, Loader, SimpleGrid, Stack, Text } from "@mantine/core";
import type { WorldResult } from "../../api";
import { FieldRow, InspectorButton, InspectorTextInput } from "../inspector";
import { StarButton } from "./StarButton";
import type { WorldSearchState } from "./useWorldSearch";

// 検索欄＋★切替＋結果グリッド＋カード（UI改善②で旧 WorldSearchPanel から共通化）。
// state は useWorldSearch（親が保持）、お気に入りも親から受領（単一の真実源は呼び出し側の方針に従う）。
// カードの主ボタンは pickLabel/onPick で注入する:
//   新規セッションタブ = 「開く」→ 確認 → 起動 ／ コンフィグ編集 = 「選択」→ loadWorldURL へセット。
// 表示の出し分け（loading → 空表示 → グリッド）も旧実装のまま移設。
export function WorldSearchView({
  search,
  pickLabel,
  onPick,
  favorites,
  isFavorited,
  onToggleFavorite,
}: {
  search: WorldSearchState;
  pickLabel: string;
  onPick: (wld: WorldResult) => void;
  favorites: WorldResult[];
  isFavorited: (recordId: string) => boolean;
  onToggleFavorite: (wld: WorldResult) => void;
}) {
  const { t } = useTranslation();
  const displayed = search.showingFavorites ? favorites : search.results;

  // 空表示（お気に入り0件 / 検索0件）と グリッド表示の出し分け。
  const showEmpty =
    !search.loading &&
    ((search.showingFavorites && favorites.length === 0) ||
      (!search.showingFavorites && search.searched && search.results.length === 0));
  const showGrid = !search.loading && displayed.length > 0;

  return (
    <Stack gap={10}>
      <FieldRow label={t("newSession.keyword")}>
        <Group gap="xs" wrap="nowrap">
          <InspectorTextInput
            value={search.keyword}
            onChange={(e) => search.setKeyword(e.currentTarget.value)}
            onKeyDown={(e) => e.key === "Enter" && void search.doSearch()}
            placeholder={t("newSession.keywordPlaceholder")}
            style={{ flex: 1, minWidth: 0 }}
          />
          <InspectorButton onClick={() => void search.doSearch()} loading={search.loading}>
            {t("newSession.search")}
          </InspectorButton>
          <StarButton
            active={search.showingFavorites}
            onClick={search.toggleFavoritesView}
            label={t("newSession.showFavorites")}
          />
        </Group>
      </FieldRow>

      {search.loading ? (
        <Center py="md">
          <Loader size="sm" />
        </Center>
      ) : showEmpty ? (
        <Text c="dimmed" size="sm" ta="center" py="md">
          {search.showingFavorites ? t("newSession.noFavorites") : t("newSession.noResults")}
        </Text>
      ) : showGrid ? (
        <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="sm">
          {displayed.map((wld) => (
            <WorldCard
              key={wld.resoniteUrl}
              world={wld}
              pickLabel={pickLabel}
              favorited={isFavorited(wld.recordId)}
              onPick={() => onPick(wld)}
              onToggleFavorite={() => onToggleFavorite(wld)}
            />
          ))}
        </SimpleGrid>
      ) : null}
    </Stack>
  );
}

// 検索結果1件のカード（サムネ＋名前＋所有者ID＋主ボタン＋★）。
// サムネ枠は常に固定高（THUMB_H）のコンテナで確保し、画像はその中を埋める（object-fit: cover）。
// これで「画像の有無・ロード前後」でカード高が変わらず、主ボタン/★ が動かない（押し間違い防止）。
const THUMB_H = 90;

function WorldCard({
  world,
  pickLabel,
  favorited,
  onPick,
  onToggleFavorite,
}: {
  world: WorldResult;
  pickLabel: string;
  favorited: boolean;
  onPick: () => void;
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
        <InspectorButton onClick={onPick} style={{ flex: 1, minWidth: 0 }}>
          {pickLabel}
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
