import { Button, Divider, Stack } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { FieldRow, InspectorCard, InspectorTextInput } from "../../components/inspector";
import type { ConfigMap } from "./configModel";
import { GeneralSection } from "./GeneralSection";
import { WorldsSection } from "./WorldsSection";

// エディタカード（detail）。タイトルは固定文言、先頭の「コンフィグ名」は編集欄（識別子＝cfg 本文とは別物）。
// 名前は親（ConfigTab）が draftName として保持し、保存時に upsert/Save As のターゲットになる。
// nameError があれば名前欄に表示し保存を抑止する（検証は親に一元化）。複製/削除は一覧の各行へ。
export function ConfigEditor({
  draftName,
  onDraftNameChange,
  nameError,
  cfg,
  onChange,
  dirty,
  saving,
  onSave,
  centralUserId,
}: {
  draftName: string;
  onDraftNameChange: (v: string) => void;
  nameError?: string;
  cfg: ConfigMap;
  onChange: (cfg: ConfigMap) => void;
  dirty: boolean;
  saving: boolean;
  onSave: () => void;
  centralUserId?: string; // customSessionId prefix の自動入力元（R12）
}) {
  const { t } = useTranslation();
  return (
    <InspectorCard title={t("config.editorTitle")}>
      <Stack gap="sm">
        <FieldRow label={t("config.nameLabel")}>
          <InspectorTextInput
            value={draftName}
            onChange={(e) => onDraftNameChange(e.currentTarget.value)}
            error={nameError}
          />
        </FieldRow>
        <GeneralSection cfg={cfg} onChange={onChange} />
        <Divider color="dark.4" />
        <WorldsSection cfg={cfg} onChange={onChange} centralUserId={centralUserId} />
        <Divider color="dark.4" />
        <Button
          fullWidth
          size="xs"
          variant={dirty ? "filled" : "default"}
          color="brand"
          disabled={!dirty || !!nameError}
          loading={saving}
          onClick={onSave}
        >
          {t("config.save")}
        </Button>
      </Stack>
    </InspectorCard>
  );
}
