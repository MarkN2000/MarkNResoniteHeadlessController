import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Collapse, Group } from "@mantine/core";
import { FieldRow, InspectorButton, InspectorTextInput } from "../../components/inspector";
import { useFavorites } from "../../components/worldsearch/useFavorites";
import { useWorldSearch } from "../../components/worldsearch/useWorldSearch";
import { WorldSearchView } from "../../components/worldsearch/WorldSearchView";

// loadWorldURL 行（URL入力＋「検索 ▾」トグル）＋ 直下の Collapse 検索パネル（UI改善②）。
// 新規セッションタブの検索UI（components/worldsearch）を共用し、カードの主ボタンは「選択」＝
// loadWorldURL へ URL をセットしてパネルを閉じる（ローカル編集のみ・保存するまで確定しないため確認なし）。
// FieldRow（リセットマーカー含む）ごと所有し、markerLabel/onMarkerClick は親（WorldsSection の
// resetProps）から転送する。検索UIは初回トグルまでマウントしない（favorites の GET も開くまで
// 走らない遅延）。一度開いたら閉じても unmount しない＝検索結果・お気に入り state を保持する。
export function WorldUrlSearch({
  label,
  markerLabel,
  onMarkerClick,
  value,
  onChange,
  onPickUrl,
}: {
  label: string;
  markerLabel?: string;
  onMarkerClick?: () => void;
  value: string;
  onChange: (v: string) => void;
  onPickUrl: (url: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [everOpened, setEverOpened] = useState(false);

  const toggle = () => {
    setOpen((o) => !o);
    setEverOpened(true);
  };

  return (
    <>
      <FieldRow label={label} markerLabel={markerLabel} onMarkerClick={onMarkerClick}>
        <Group gap="xs" wrap="nowrap">
          <InspectorTextInput
            value={value}
            onChange={(e) => onChange(e.currentTarget.value)}
            placeholder="resrec://..."
            style={{ flex: 1, minWidth: 0 }}
          />
          <InspectorButton onClick={toggle} aria-expanded={open}>
            {t("newSession.search")} {open ? "▴" : "▾"}
          </InspectorButton>
        </Group>
      </FieldRow>
      {everOpened && (
        <Collapse in={open}>
          <Box pt={4}>
            <SearchBody
              onPick={(url) => {
                onPickUrl(url);
                setOpen(false);
              }}
            />
          </Box>
        </Collapse>
      )}
    </>
  );
}

// Collapse の中身（初回オープン時にマウント）。検索 state とお気に入りをここで保持する。
function SearchBody({ onPick }: { onPick: (url: string) => void }) {
  const { t } = useTranslation();
  const search = useWorldSearch();
  const fav = useFavorites();
  return (
    <WorldSearchView
      search={search}
      pickLabel={t("config.worldSearchPick")}
      onPick={(wld) => onPick(wld.resoniteUrl)}
      favorites={fav.favorites}
      isFavorited={fav.isFavorited}
      onToggleFavorite={fav.toggle}
    />
  );
}
