import { NumberInput, Select, Textarea, TextInput } from "@mantine/core";
import type { NumberInputProps, SelectProps, TextareaProps, TextInputProps } from "@mantine/core";
import { FIELD_INPUT_STYLES, FIELD_SELECT_STYLES, FIELD_TEXTAREA_STYLES, SELECT_DOWN_ICON } from "./FieldRow";

// インスペクタ風の入力欄ラッパ。サイズ/variant/スタイル/▼アイコンを内蔵し、各タブで使い回す。
// ルール（§3.7 由来）: キーボード入力欄（TextInput/NumberInput/Textarea）は縁取りあり、
// プルダウン（Select）は縁取りなし・グレー fill＋▼。固定スタイルは spread の後に置いて常に効かせる。

export function InspectorTextInput(props: TextInputProps) {
  return <TextInput size="xs" variant="filled" {...props} styles={FIELD_INPUT_STYLES} />;
}

export function InspectorNumberInput(props: NumberInputProps) {
  return <NumberInput size="xs" variant="filled" {...props} styles={FIELD_INPUT_STYLES} />;
}

export function InspectorTextarea(props: TextareaProps) {
  return <Textarea size="xs" variant="filled" autosize minRows={2} {...props} styles={FIELD_TEXTAREA_STYLES} />;
}

export function InspectorSelect(props: SelectProps) {
  return (
    <Select
      size="xs"
      variant="filled"
      allowDeselect={false}
      comboboxProps={{ withinPortal: true }}
      {...props}
      rightSection={SELECT_DOWN_ICON}
      rightSectionPointerEvents="none"
      styles={FIELD_SELECT_STYLES}
    />
  );
}
