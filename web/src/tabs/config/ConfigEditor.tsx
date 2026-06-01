import { Button, Divider, Stack } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { InspectorCard } from "../../components/inspector";
import type { ConfigMap } from "./configModel";
import { GeneralSection } from "./GeneralSection";
import { WorldsSection } from "./WorldsSection";

// エディタカード（detail）。ヘッダは config 名のみ（複製/削除は一覧の各行へ移動）。
// 本文 = 全体設定＋ワールド＋保存。親（ConfigTab）が key={name} で再マウントするため、
// バッファ付きフィールド/タブ状態は config 単位でリセットされる。
export function ConfigEditor({
  name,
  cfg,
  onChange,
  dirty,
  saving,
  onSave,
}: {
  name: string;
  cfg: ConfigMap;
  onChange: (cfg: ConfigMap) => void;
  dirty: boolean;
  saving: boolean;
  onSave: () => void;
}) {
  const { t } = useTranslation();
  return (
    <InspectorCard title={name}>
      <Stack gap="sm">
        <GeneralSection cfg={cfg} onChange={onChange} />
        <Divider color="dark.4" />
        <WorldsSection cfg={cfg} onChange={onChange} />
        <Divider color="dark.4" />
        <Button
          fullWidth
          size="xs"
          variant={dirty ? "filled" : "default"}
          color="brand"
          disabled={!dirty}
          loading={saving}
          onClick={onSave}
        >
          {t("config.save")}
        </Button>
      </Stack>
    </InspectorCard>
  );
}
