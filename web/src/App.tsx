import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Badge,
  Button,
  Center,
  Group,
  Paper,
  PasswordInput,
  ScrollArea,
  Stack,
  Text,
  TextInput,
  Title,
} from "@mantine/core";
import { setLanguage } from "./i18n";
import * as api from "./api";
import type { Status, LogLine } from "./api";

export default function App() {
  const { t, i18n } = useTranslation();
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [pw, setPw] = useState("");
  const [loginErr, setLoginErr] = useState("");
  const [status, setStatus] = useState<Status | null>(null);
  const [logs, setLogs] = useState<LogLine[]>([]);
  const [cmd, setCmd] = useState("");
  const esRef = useRef<EventSource | null>(null);
  const seenSeq = useRef<Set<number>>(new Set());
  const logEndRef = useRef<HTMLDivElement | null>(null);

  // 初回: 既存セッションを確認
  useEffect(() => {
    api.getStatus().then((s) => {
      if (s) {
        setStatus(s);
        setAuthed(true);
      } else {
        setAuthed(false);
      }
    });
  }, []);

  // 認証済みになったら SSE 接続
  useEffect(() => {
    if (authed !== true) return;
    const es = new EventSource("/api/v1/events");
    esRef.current = es;
    es.addEventListener("status", (e) => {
      setStatus(JSON.parse((e as MessageEvent).data) as Status);
    });
    es.addEventListener("log", (e) => {
      const line = JSON.parse((e as MessageEvent).data) as LogLine;
      if (seenSeq.current.has(line.seq)) return; // seqで重複排除
      seenSeq.current.add(line.seq);
      setLogs((prev) => {
        const next = [...prev, line];
        return next.length > 1000 ? next.slice(next.length - 1000) : next;
      });
    });
    return () => {
      es.close();
      esRef.current = null;
    };
  }, [authed]);

  // ログ末尾へ自動スクロール
  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "auto" });
  }, [logs]);

  async function doLogin() {
    setLoginErr("");
    const r = await api.login(pw);
    if (r.ok) {
      setPw("");
      seenSeq.current.clear();
      setLogs([]);
      setStatus(await api.getStatus());
      setAuthed(true);
    } else {
      setLoginErr(r.status === 429 ? t("login.rateLimited") : t("login.error"));
    }
  }

  async function doLogout() {
    await api.logout();
    esRef.current?.close();
    setAuthed(false);
    setStatus(null);
    setLogs([]);
  }

  function submitCmd(e: React.FormEvent) {
    e.preventDefault();
    const c = cmd.trim();
    if (!c) return;
    void api.sendCommand(c);
    setCmd("");
  }

  function toggleLang() {
    setLanguage(i18n.language === "ja" ? "en" : "ja");
  }

  if (authed === null) {
    return <Center h="100%"><Text c="dimmed">…</Text></Center>;
  }

  if (!authed) {
    return (
      <Center h="100%" p="md">
        <Paper w="100%" maw={380} p="lg" radius="md" withBorder>
          <Group justify="space-between" mb="md">
            <Title order={3}>{t("login.title")}</Title>
            <Button variant="subtle" size="xs" onClick={toggleLang}>
              {t("langToggle")}
            </Button>
          </Group>
          <PasswordInput
            value={pw}
            onChange={(e) => setPw(e.currentTarget.value)}
            onKeyDown={(e) => e.key === "Enter" && doLogin()}
            placeholder={t("login.password")}
            mb="sm"
          />
          <Button fullWidth onClick={doLogin}>
            {t("login.submit")}
          </Button>
          {loginErr && (
            <Text c="red" size="sm" mt="sm">
              {loginErr}
            </Text>
          )}
        </Paper>
      </Center>
    );
  }

  const st = status?.state ?? "stopped";
  const badgeColor = st === "running" ? "green" : st === "stopped" ? "red" : "yellow";
  const colorOf = (kind: string) =>
    kind === "err" ? "red.4" : kind === "sys" ? "blue.4" : kind === "cmd" ? "green.4" : "gray.4";

  return (
    <Stack h="100%" p="md" gap="xs" maw={960} mx="auto">
      <Group>
        <Badge color={badgeColor} size="lg" variant="light">
          {t("state.label")}: {t(`state.${st}`)}
          {status?.ready ? ` (${t("dashboard.ready")})` : ""}
          {status?.pid ? ` · pid ${status.pid}` : ""}
        </Badge>
        <div style={{ flex: 1 }} />
        <Button color="green" onClick={() => void api.start()}>
          {t("dashboard.start")}
        </Button>
        <Button color="red" onClick={() => void api.stop()}>
          {t("dashboard.stop")}
        </Button>
        <Button variant="default" onClick={toggleLang}>
          {t("langToggle")}
        </Button>
        <Button variant="default" onClick={doLogout}>
          {t("dashboard.logout")}
        </Button>
      </Group>

      <ScrollArea style={{ flex: 1, minHeight: 0 }} bg="black" p="xs">
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
        <div ref={logEndRef} />
      </ScrollArea>

      <form onSubmit={submitCmd}>
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
