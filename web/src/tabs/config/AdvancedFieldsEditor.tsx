import { useState } from "react";
import { ActionIcon, Box, Group, Stack, Switch, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { InspectorNumberInput, InspectorSelect, InspectorTextInput } from "../../components/inspector";
import { asBool, asNum, asStr, getStringArray } from "./configModel";
import type { FieldDef } from "./fieldCatalog";
import { defForKey, extraKeysInOrder, initialValueFor } from "./fieldCatalog";
import { RawJsonInput, StringListInput } from "./fields";

// ③「詳細フィールド」: 専用フォーム（①②）に無いキーを 1 か所で面倒みる汎用エディタ。
//   - map に存在する非 dedicated キーを行表示（型別ウィジェット・左端×でキー削除）
//   - ドロップダウンは「カタログのうち未追加のキー」のみ列挙（重複・誤キー不可）
//   - カタログ外の未知キーは json（生 JSON）で温存表示
// obj は不透明 map（top=cfg / world=1ワールド）。set/del は {...obj} で新オブジェクトを作り onChange。
// 内部 state を持つ子（RawJsonInput）があるため、呼び出し側は key（ワールド index 等）で再マウントする。
export function AdvancedFieldsEditor({
  obj,
  onChange,
  dedicated,
  catalog,
}: {
  obj: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  dedicated: ReadonlySet<string>;
  catalog: readonly FieldDef[];
}) {
  const { t } = useTranslation();
  const [toAdd, setToAdd] = useState<string | null>(null);

  const set = (key: string, value: unknown) => onChange({ ...obj, [key]: value });
  const del = (key: string) => {
    const next = { ...obj };
    delete next[key];
    onChange(next);
  };

  const extraKeys = extraKeysInOrder(obj, dedicated, catalog);
  const addable = catalog.filter((d) => !(d.key in obj)).map((d) => d.key);

  const add = () => {
    if (!toAdd) return;
    set(toAdd, initialValueFor(defForKey(catalog, toAdd).type));
    setToAdd(null);
  };

  // ラベルは i18n（config.<key>）。未定義なら key 名そのもの（未知キー用）。
  const label = (key: string) => t(`config.${key}`, { defaultValue: key });

  // テキスト欄: 空文字（空白のみ含む）は null（未設定）として確定。
  const setText = (key: string, v: string) => set(key, v.trim() === "" ? null : v);

  const isMultiline = (key: string) => {
    const ty = defForKey(catalog, key).type;
    return ty === "strarray" || ty === "json";
  };

  const renderWidget = (key: string) => {
    const def = defForKey(catalog, key);
    const v = obj[key];
    switch (def.type) {
      case "bool":
        return <Switch checked={asBool(v)} onChange={(e) => set(key, e.currentTarget.checked)} />;
      case "int":
        return (
          <InspectorNumberInput
            value={asNum(v)}
            onChange={(n) => set(key, n === "" ? null : Number(n))}
            min={0}
            allowNegative={false}
          />
        );
      case "enum":
        return <InspectorSelect data={[...(def.enum ?? [])]} value={asStr(v)} onChange={(val) => val && set(key, val)} />;
      case "strarray":
        return (
          <StringListInput
            items={getStringArray(v)}
            onChange={(items) => set(key, items.length ? items : null)}
            addLabel={t("config.add")}
            placeholder={t("config.listItemPlaceholder")}
          />
        );
      case "json":
        return <RawJsonInput initial={v} onCommit={(parsed) => set(key, parsed)} invalidLabel={t("config.invalidJson")} />;
      case "string":
      default:
        return <InspectorTextInput value={asStr(v)} onChange={(e) => setText(key, e.currentTarget.value)} />;
    }
  };

  return (
    <Stack gap={6}>
      <Text size="xs" c="dimmed">
        {t("config.advancedFieldsTitle")}
      </Text>
      {/* 追加コントロール（未追加キーのドロップダウン＋＋）。 */}
      <Group gap={4} wrap="nowrap">
        <Box style={{ flex: 1, minWidth: 0 }}>
          <InspectorSelect
            data={addable.map((k) => ({ value: k, label: label(k) }))}
            value={toAdd}
            placeholder={addable.length ? t("config.addFieldPlaceholder") : t("config.noAddableFields")}
            disabled={addable.length === 0}
            onChange={setToAdd}
          />
        </Box>
        <ActionIcon
          size="lg"
          variant="light"
          color="gray"
          aria-label={t("config.add")}
          title={t("config.add")}
          disabled={!toAdd}
          onClick={add}
        >
          ＋
        </ActionIcon>
      </Group>
      {/* 追加済みフィールドの行（左端×＝キー削除）。 */}
      {extraKeys.map((key) => (
        <Group key={key} wrap="nowrap" gap="xs" align={isMultiline(key) ? "start" : "center"} style={{ minHeight: 34 }}>
          <Group gap={6} wrap="nowrap" style={{ width: 175, flexShrink: 0, paddingTop: isMultiline(key) ? 6 : 0 }}>
            <ActionIcon
              size="sm"
              variant="subtle"
              color="red"
              aria-label={t("config.deleteField")}
              title={t("config.deleteField")}
              onClick={() => del(key)}
            >
              ×
            </ActionIcon>
            <Text size="sm" c="dark.1" style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
              {label(key)}
            </Text>
          </Group>
          <Box style={{ flex: 1, minWidth: 0 }}>{renderWidget(key)}</Box>
        </Group>
      ))}
    </Stack>
  );
}
