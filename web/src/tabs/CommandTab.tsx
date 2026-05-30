import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Group, ScrollArea, Stack, Text, TextInput } from "@mantine/core";
import type { LogLine } from "../api";

// コマンドタブ: SSE ライブログ + コマンド直送（上級者用）。docs §3.3 #7。
export function CommandTab({ logs, onSend }: { logs: LogLine[]; onSend: (cmd: string) => void }) {
  const { t } = useTranslation();
  const [cmd, setCmd] = useState("");
  const endRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "auto" });
  }, [logs]);

  // err=危険/赤, sys=通知/黄, cmd=入力/シアン, out=本文/light
  const colorOf = (kind: string) =>
    kind === "err" ? "red" : kind === "sys" ? "yellow" : kind === "cmd" ? "brand" : "dark.1";

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const c = cmd.trim();
    if (!c) return;
    onSend(c);
    setCmd("");
  }

  return (
    <Stack h="100%" gap="xs">
      <ScrollArea style={{ flex: 1, minHeight: 0 }} bg="dark.9" p="xs" styles={{ root: { borderRadius: 8 } }}>
        {logs.map((l) => (
          <Text
            key={l.seq}
            ff="monospace"
            size="xs"
            c={colorOf(l.kind)}
            style={{ whiteSpace: "pre-wrap", lineHeight: 1.4 }}
          >
            {l.text}
          </Text>
        ))}
        <div ref={endRef} />
      </ScrollArea>

      <form onSubmit={submit}>
        <Group gap="xs">
          <TextInput
            style={{ flex: 1 }}
            value={cmd}
            onChange={(e) => setCmd(e.currentTarget.value)}
            placeholder={t("dashboard.commandPlaceholder")}
            ff="monospace"
            autoComplete="off"
          />
          <Button type="submit" variant="default">
            {t("dashboard.send")}
          </Button>
        </Group>
      </form>
    </Stack>
  );
}
