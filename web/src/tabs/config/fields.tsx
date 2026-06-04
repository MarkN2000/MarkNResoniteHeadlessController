import { useState } from "react";
import { ActionIcon, Box, Group, Stack, Text } from "@mantine/core";
import type { NumberInputProps } from "@mantine/core";
import * as api from "../../api";
import { InspectorNumberInput, InspectorSelect, InspectorTextarea, InspectorTextInput, RowIconButton } from "../../components/inspector";
import { joinCustomSessionId, splitCustomSessionId } from "./configModel";

// -1=無効 のセンチネル数値入力。下限を -1 に固定（InspectorNumberInput の strict で -1 未満は打てない）。
// 整数のみ・空欄→-1 の正規化は呼び出し側（WorldsSection の sentinelW）が担う。-1 の下限はここ1箇所。
export function SentinelNumberInput(props: NumberInputProps) {
  return <InspectorNumberInput min={-1} {...props} />;
}

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
// autoPrefix（中央アカウントの解決済 UserID・R12）: 既存 prefix が空のときだけ自動シード（上書き可）。
// 表示のみ初期化し、map へは編集（onChange）時に初めて書く＝未編集なら customSessionId は未設定のまま。
export function CustomSessionIdInput({
  initial,
  onChange,
  autoPrefix,
}: {
  initial: string;
  onChange: (v: string) => void;
  autoPrefix?: string;
}) {
  const s0 = splitCustomSessionId(initial);
  const [prefix, setPrefix] = useState(s0.prefix || autoPrefix || "");
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

// 文字列配列の add/remove リスト（配列の生 JSON 編集を避ける）。
// allowedUrlHosts / autoInviteUsernames / inviteRequestHandlerUsernames / parentSessionIds で共用。
// committed なリストは props 駆動（draft のみ内部 state）＝リセット/外部変更に追従できる。
export function StringListInput({
  items,
  onChange,
  addLabel,
  placeholder,
}: {
  items: string[];
  onChange: (items: string[]) => void;
  addLabel: string;
  placeholder: string;
}) {
  const [draft, setDraft] = useState("");
  const add = () => {
    const h = draft.trim();
    if (!h) return;
    onChange([...items, h]);
    setDraft("");
  };
  return (
    <Stack gap={4}>
      {items.map((h, i) => (
        <Group key={i} gap={4} wrap="nowrap">
          <Text size="xs" c="dark.0" style={{ flex: 1, minWidth: 0, wordBreak: "break-all" }}>
            {h}
          </Text>
          <RowIconButton color="red" label="×" onClick={() => onChange(items.filter((_, idx) => idx !== i))}>
            ×
          </RowIconButton>
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

// defaultUserRoles 用: 「ユーザー名 → ロール」の追加式エディタ。値は { username: role } の object
// （スキーマの additionalProperties:string）。ロールは標準5種(api.ROLES)から選択。
// 空ユーザー名の行は確定 object から除外し、同名ユーザーは後勝ち（object 上書き）。新規行の既定ロールは
// 安全側の "Guest"。入力途中の行を保持するため内部 state（確定値のみ onChange へ集約）。key で再シード。
export function RolePairsInput({
  initial,
  onChange,
  userPlaceholder,
  addLabel,
}: {
  initial: unknown;
  onChange: (next: Record<string, string> | null) => void;
  userPlaceholder: string;
  addLabel: string;
}) {
  const seed = (v: unknown): { user: string; role: string }[] =>
    v && typeof v === "object" && !Array.isArray(v)
      ? Object.entries(v as Record<string, unknown>).map(([user, role]) => ({
          user,
          role: typeof role === "string" ? role : "Guest",
        }))
      : [];
  const [rows, setRows] = useState<{ user: string; role: string }[]>(() => seed(initial));
  const commit = (next: { user: string; role: string }[]) => {
    setRows(next);
    const obj: Record<string, string> = {};
    for (const r of next) {
      const u = r.user.trim();
      if (u) obj[u] = r.role; // 同名は後勝ち
    }
    onChange(Object.keys(obj).length ? obj : null);
  };
  const update = (i: number, patch: Partial<{ user: string; role: string }>) =>
    commit(rows.map((r, idx) => (idx === i ? { ...r, ...patch } : r)));
  const remove = (i: number) => commit(rows.filter((_, idx) => idx !== i));
  const add = () => commit([...rows, { user: "", role: "Guest" }]);
  return (
    <Stack gap={4}>
      {rows.map((r, i) => (
        <Group key={i} gap={4} wrap="nowrap">
          <Box style={{ flex: 1, minWidth: 0 }}>
            <InspectorTextInput
              value={r.user}
              placeholder={userPlaceholder}
              onChange={(e) => update(i, { user: e.currentTarget.value })}
            />
          </Box>
          <Box style={{ width: 120, flexShrink: 0 }}>
            <InspectorSelect data={[...api.ROLES]} value={r.role} onChange={(v) => v && update(i, { role: v })} />
          </Box>
          <RowIconButton color="red" label="×" onClick={() => remove(i)}>
            ×
          </RowIconButton>
        </Group>
      ))}
      <Group gap={4} wrap="nowrap">
        <ActionIcon size="lg" variant="light" color="gray" aria-label={addLabel} title={addLabel} onClick={add}>
          ＋
        </ActionIcon>
      </Group>
    </Stack>
  );
}

// json 型／未知キー用: 生 JSON を編集。表示は内部 text、確定は JSON.parse して onCommit。
// パース不可なら確定せず（最後の正常値を保持）エラー文言を出す。空文字は null として確定。
// 内部 state のため key で再シードする（ワールド切替時など）。
export function RawJsonInput({
  initial,
  onCommit,
  invalidLabel,
}: {
  initial: unknown;
  onCommit: (parsed: unknown) => void;
  invalidLabel: string;
}) {
  const [text, setText] = useState(() => (initial == null ? "" : JSON.stringify(initial)));
  const [error, setError] = useState<string | undefined>(undefined);
  return (
    <InspectorTextarea
      value={text}
      error={error}
      minRows={1}
      onChange={(e) => {
        const v = e.currentTarget.value;
        setText(v);
        if (v.trim() === "") {
          setError(undefined);
          onCommit(null);
          return;
        }
        try {
          const parsed: unknown = JSON.parse(v);
          setError(undefined);
          onCommit(parsed);
        } catch {
          setError(invalidLabel); // 確定しない（最後の正常値を保持）
        }
      }}
    />
  );
}
