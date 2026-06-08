import { useState } from "react";
import { Box, Group, Stack, Text } from "@mantine/core";
import type { NumberInputProps } from "@mantine/core";
import * as api from "../../api";
import {
  AddIconButton,
  InspectorNumberInput,
  InspectorSelect,
  InspectorTextarea,
  InspectorTextInput,
  RowIconButton,
} from "../../components/inspector";
import { useIsNarrow } from "../../hooks/useIsNarrow";
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

// 1行=セル配列の in-place 行エディタ（許可ホスト=1列・デフォルトロール=2列 等を統一）。
// 既存項目もそのまま入力欄で、＋で空行追加・×で行削除・その場編集可。入力途中を保持するため内部 state
// （buffered）で、変更のたびに onRowsChange(rows) を呼ぶ。空キー行の除外や保存形(string[]/object)への
// 直列化は呼び出し側アダプタ（StringListInput / RolePairsInput）が担う。key で再シードする。
export type CellSpec =
  | { kind: "text"; placeholder?: string }
  | { kind: "select"; options: readonly string[]; width?: number; addDefault: string };

export function RowsEditor({
  columns,
  initialRows,
  onRowsChange,
  addLabel,
}: {
  columns: CellSpec[];
  initialRows: string[][];
  onRowsChange: (rows: string[][]) => void;
  addLabel: string;
}) {
  const narrow = useIsNarrow();
  const [rows, setRows] = useState<string[][]>(() => initialRows);
  const commit = (next: string[][]) => {
    setRows(next);
    onRowsChange(next);
  };
  const update = (ri: number, ci: number, val: string) =>
    commit(rows.map((r, i) => (i === ri ? r.map((c, j) => (j === ci ? val : c)) : r)));
  const remove = (ri: number) => commit(rows.filter((_, i) => i !== ri));
  // 新規行の初期値: text=空、select=addDefault。
  const add = () => commit([...rows, columns.map((c) => (c.kind === "select" ? c.addDefault : ""))]);

  // 1セルの入力ウィジェット（text/select）。ラッパ Box は呼び出し側（PC=固定幅/モバイル=全幅）が持つ。
  const cellInput = (col: CellSpec, ci: number, row: string[], ri: number) => {
    if (col.kind === "text") {
      return (
        <InspectorTextInput
          value={row[ci] ?? ""}
          placeholder={col.placeholder}
          onChange={(e) => update(ri, ci, e.currentTarget.value)}
        />
      );
    }
    // 候補外の値（既存 config のカスタムロール等）も表示できるよう data に補う。
    const cur = row[ci] ?? col.addDefault;
    const data = col.options.includes(cur) ? [...col.options] : [...col.options, cur];
    return <InspectorSelect data={data} value={cur} onChange={(v) => v && update(ri, ci, v)} />;
  };

  const deleteButton = (ri: number) => (
    <RowIconButton color="red" label="×" onClick={() => remove(ri)}>
      ×
    </RowIconButton>
  );

  // 行の描画。モバイル かつ 複数列（select を含む defaultUserRoles 等）のときだけ「カード枠＋2段」へ。
  //   1段目=最後尾以外の列（=名前）を全幅 / 2段目=最後の列（=ロール）＋削除× を横並び。
  // 1列（テキストのみ）や PC では従来の横一列を維持（元々あふれないため）。狭幅判定は [[useIsNarrow]]。
  const renderRow = (row: string[], ri: number) => {
    if (narrow && columns.length > 1) {
      const last = columns.length - 1;
      return (
        <Box
          key={ri}
          style={{
            border: "1px solid var(--mantine-color-dark-4)",
            borderRadius: "var(--mantine-radius-md)",
            padding: 6,
          }}
        >
          <Stack gap={4}>
            {columns.slice(0, last).map((col, ci) => (
              <Box key={ci} style={{ width: "100%" }}>
                {cellInput(col, ci, row, ri)}
              </Box>
            ))}
            <Group gap={4} wrap="nowrap">
              <Box style={{ flex: 1, minWidth: 0 }}>{cellInput(columns[last], last, row, ri)}</Box>
              {deleteButton(ri)}
            </Group>
          </Stack>
        </Box>
      );
    }
    return (
      <Group key={ri} gap={4} wrap="nowrap">
        {columns.map((col, ci) =>
          col.kind === "text" ? (
            <Box key={ci} style={{ flex: 1, minWidth: 0 }}>
              {cellInput(col, ci, row, ri)}
            </Box>
          ) : (
            <Box key={ci} style={{ width: col.width ?? 120, flexShrink: 0 }}>
              {cellInput(col, ci, row, ri)}
            </Box>
          ),
        )}
        {deleteButton(ri)}
      </Group>
    );
  };

  return (
    <Stack gap={4}>
      {rows.map((row, ri) => renderRow(row, ri))}
      <Group gap={4} wrap="nowrap">
        <AddIconButton label={addLabel} onClick={add} />
      </Group>
    </Stack>
  );
}

// 文字列配列の行エディタ（1列）。allowedUrlHosts / autoInviteUsernames / inviteRequestHandlerUsernames /
// parentSessionIds / ③の文字列配列で共用。先頭セル空（trim後）の行は除外。重複は許容（dedupしない）。
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
  return (
    <RowsEditor
      columns={[{ kind: "text", placeholder }]}
      initialRows={items.map((s) => [s])}
      onRowsChange={(rows) => onChange(rows.map((r) => (r[0] ?? "").trim()).filter((s) => s !== ""))}
      addLabel={addLabel}
    />
  );
}

// defaultUserRoles 用の行エディタ（2列＝ユーザー名 text ＋ ロール select）。値は { user: role } の object
// （スキーマの additionalProperties:string）。先頭セル（ユーザー名）空の行は除外・同名は後勝ち。
// 新規行の既定ロールは "Admin"（主用途が管理者権限付与のため）。読み込み時の非文字列ロールは "Guest" に丸める。
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
  const initialRows: string[][] =
    initial && typeof initial === "object" && !Array.isArray(initial)
      ? Object.entries(initial as Record<string, unknown>).map(([u, r]) => [u, typeof r === "string" ? r : "Guest"])
      : [];
  return (
    <RowsEditor
      columns={[
        { kind: "text", placeholder: userPlaceholder },
        { kind: "select", options: api.ROLES, width: 120, addDefault: "Admin" },
      ]}
      initialRows={initialRows}
      onRowsChange={(rows) => {
        const obj: Record<string, string> = {};
        for (const r of rows) {
          const u = (r[0] ?? "").trim();
          if (u) obj[u] = r[1] ?? "Admin"; // 同名は後勝ち
        }
        onChange(Object.keys(obj).length ? obj : null);
      }}
      addLabel={addLabel}
    />
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
