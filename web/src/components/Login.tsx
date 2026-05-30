import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Center, Group, Paper, PasswordInput, Select, Text, Title } from "@mantine/core";
import { LANGUAGES, setLanguage } from "../i18n";
import * as api from "../api";

// ログインカード（ロゴ + password + ボタン）。失敗はカード内赤表示。
export function Login({ onAuthed }: { onAuthed: () => void }) {
  const { t, i18n } = useTranslation();
  const [pw, setPw] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit() {
    if (busy) return;
    setErr("");
    setBusy(true);
    const r = await api.login(pw);
    setBusy(false);
    if (r.ok) {
      setPw("");
      onAuthed();
    } else {
      setErr(r.status === 429 ? t("login.rateLimited") : t("login.error"));
    }
  }

  return (
    <Center h="100%" p="md">
      <Paper w="100%" maw={380} p="lg" radius="md" bg="dark.6">
        <Group justify="space-between" mb="md">
          <Title order={3}>{t("login.title")}</Title>
          <Select
            data={LANGUAGES.map((l) => ({ value: l.code, label: l.label }))}
            value={i18n.language}
            onChange={(v) => v && setLanguage(v)}
            size="xs"
            w={110}
            allowDeselect={false}
            comboboxProps={{ withinPortal: true }}
            aria-label="language"
          />
        </Group>
        <PasswordInput
          value={pw}
          onChange={(e) => setPw(e.currentTarget.value)}
          onKeyDown={(e) => e.key === "Enter" && submit()}
          placeholder={t("login.password")}
          mb="sm"
          autoFocus
        />
        <Button fullWidth onClick={submit} loading={busy}>
          {t("login.submit")}
        </Button>
        {err && (
          <Text c="red" size="sm" mt="sm">
            {err}
          </Text>
        )}
      </Paper>
    </Center>
  );
}
