import { useState } from "react";
import { ActionIcon, Group, Stack, Text } from "@mantine/core";
import { InspectorTextInput } from "../../components/inspector";
import { joinCustomSessionId, splitCustomSessionId } from "./configModel";

// config 固有のフィールド widget。配列/結合値を文字列フォームで編集する際の「タイプ途中の状態」
// 喪失を防ぐため内部 state を持つ（map へは確定値のみ書く）。呼び出し側は key で再シードする
// （ワールドタブ切替・config 切替時）。

// 配列等を文字列で編集するバッファ付き入力。表示は内部 text、確定は parse して onCommit。
export function BufferedTextInput({
  initial,
  parse,
  onCommit,
  placeholder,
}: {
  initial: string;
  parse: (s: string) => unknown;
  onCommit: (parsed: unknown) => void;
  placeholder?: string;
}) {
  const [text, setText] = useState(initial);
  return (
    <InspectorTextInput
      value={text}
      placeholder={placeholder}
      onChange={(e) => {
        const v = e.currentTarget.value;
        setText(v);
        onCommit(parse(v));
      }}
    />
  );
}

// customSessionId の prefix/suffix ビルダー（v1 互換・最初の ':' で結合）。
// prefix のみ等の途中状態を保持するため内部 state。key={worldIndex} で再シードする。
export function CustomSessionIdInput({ initial, onChange }: { initial: string; onChange: (v: string) => void }) {
  const s0 = splitCustomSessionId(initial);
  const [prefix, setPrefix] = useState(s0.prefix);
  const [suffix, setSuffix] = useState(s0.suffix);
  const emit = (p: string, s: string) => {
    setPrefix(p);
    setSuffix(s);
    onChange(joinCustomSessionId(p, s));
  };
  return (
    <Group gap={4} wrap="nowrap" align="center">
      <InspectorTextInput value={prefix} placeholder="U-…" onChange={(e) => emit(e.currentTarget.value, suffix)} />
      <Text size="sm" c="dark.1">
        :
      </Text>
      <InspectorTextInput value={suffix} placeholder="(空で無効)" onChange={(e) => emit(prefix, e.currentTarget.value)} />
    </Group>
  );
}

// allowedUrlHosts の add/remove リスト（配列の生 JSON 編集を避ける）。
export function HostListInput({
  hosts,
  onChange,
  addLabel,
  placeholder,
}: {
  hosts: string[];
  onChange: (hosts: string[]) => void;
  addLabel: string;
  placeholder: string;
}) {
  const [draft, setDraft] = useState("");
  const add = () => {
    const h = draft.trim();
    if (!h) return;
    onChange([...hosts, h]);
    setDraft("");
  };
  return (
    <Stack gap={4}>
      {hosts.map((h, i) => (
        <Group key={i} gap={4} wrap="nowrap">
          <Text size="xs" c="dark.0" style={{ flex: 1, minWidth: 0, wordBreak: "break-all" }}>
            {h}
          </Text>
          <ActionIcon
            size="sm"
            variant="subtle"
            color="red"
            aria-label="×"
            onClick={() => onChange(hosts.filter((_, idx) => idx !== i))}
          >
            ×
          </ActionIcon>
        </Group>
      ))}
      <Group gap={4} wrap="nowrap">
        <InspectorTextInput
          value={draft}
          placeholder={placeholder}
          onChange={(e) => setDraft(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
        />
        <ActionIcon size="lg" variant="light" color="gray" aria-label={addLabel} title={addLabel} onClick={add}>
          ＋
        </ActionIcon>
      </Group>
    </Stack>
  );
}
