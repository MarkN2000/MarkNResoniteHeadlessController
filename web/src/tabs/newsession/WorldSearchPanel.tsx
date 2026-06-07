import { useTranslation } from "react-i18next";
import * as api from "../../api";
import type { WorldResult } from "../../api";
import { InspectorCard } from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import { useWorldSearch } from "../../components/worldsearch/useWorldSearch";
import { WorldSearchView } from "../../components/worldsearch/WorldSearchView";

// ワールド検索 → 検索結果から起動／お気に入り保存（phase-7-spec §3.12）。
// 検索UI・state は components/worldsearch へ共通化（UI改善②・コンフィグ編集の WorldUrlSearch と共用）。
// ここに残るのは新規セッションタブ固有の部分のみ:
//   件数つきカードタイトル・起動確認モーダル・startWorldURL（resrec:// 起動）。
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
  const search = useWorldSearch();
  const confirm = useConfirm();

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

  const title = search.showingFavorites
    ? `${t("newSession.favorites")} (${favorites.length})`
    : search.searched && !search.loading
      ? `${t("newSession.searchTitle")} (${search.results.length})`
      : t("newSession.searchTitle");

  return (
    <InspectorCard title={title}>
      <WorldSearchView
        search={search}
        pickLabel={t("newSession.start")}
        onPick={askStart}
        favorites={favorites}
        isFavorited={isFavorited}
        onToggleFavorite={onToggleFavorite}
      />
      <ConfirmHost confirm={confirm} />
    </InspectorCard>
  );
}
