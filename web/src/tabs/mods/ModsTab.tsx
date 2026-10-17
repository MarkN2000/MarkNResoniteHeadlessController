import { useCallback, useEffect, useRef, useState } from "react";
import { ActionIcon, Alert, Box, Button, Divider, Group, Loader, Menu, Modal, ScrollArea, Select, Stack, Switch, Text, TextInput } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import { InspectorButton, InspectorCard } from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";

const RML_REPO = "resonite-modding-group/ResoniteModLoader";
type Dialog = { kind: "upload" | "github" | "source"; entry?: api.ModEntry; check?: api.ModCheck };
type CheckResult = { fingerprint: string; value?: api.ModCheck; failed?: boolean };
const fingerprint = (e?: api.ModEntry) => JSON.stringify([e?.current, e?.updateSource]);

export function ModsTab({ status }: { status: api.Status | null }) {
  const { t, i18n } = useTranslation();
  const [info, setInfo] = useState<api.ModsInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [checks, setChecks] = useState<Record<string, CheckResult>>({});
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const request = useRef(0);
  const confirm = useConfirm();
  const action = useAsyncAction();
  const load = useCallback(async () => {
    const current = ++request.current;
    const r = await api.getMods();
    if (current !== request.current) return;
    setError(r.ok ? null : r.error ?? "");
    if (r.ok) setInfo(r.data as api.ModsInfo);
  }, []);
  useEffect(() => { void load(); return () => { request.current++; }; }, [load, status?.state]);
  useVisiblePolling(load, 5000);
  const editable = status?.state === "stopped" && !!info?.canModify && error === null && !action.busy && !confirm.busy;
  const rml = info?.entries.find((e) => e.id === "rml");

  async function change(fn: () => Promise<api.WriteResult>) {
    const r = await fn();
    if (r.ok) { setChecks({}); setDialog(null); }
    await load();
    return r;
  }
  const runChange = (fn: () => Promise<api.WriteResult>) => action.run(() => change(fn));

  function check(e?: api.ModEntry) {
    const id = e?.id ?? "rml";
    void action.run(async () => {
      const r = await api.checkMod(id);
      setChecks((prev) => ({ ...prev, [id]: { fingerprint: fingerprint(e), value: r.ok ? r.data as api.ModCheck : undefined, failed: !r.ok } }));
      return r;
    });
  }

  function update(e: api.ModEntry, c: api.ModCheck) {
    if (!c.asset) { setDialog({ kind: "github", entry: e, check: c }); return; }
    void runChange(() => api.downloadMod({ id: e.id, repo: c.repo, tag: c.tag, asset: c.asset }));
  }

  function installRML() {
    void runChange(async () => {
      const r = await api.checkMod("rml");
      if (!r.ok) return r;
      const c = r.data as api.ModCheck;
      return api.downloadMod({ id: "rml", repo: RML_REPO, tag: c.tag, asset: "ResoniteModLoader.dll" });
    });
  }

  function row(e: api.ModEntry) {
    const isRML = e.id === "rml";
    const name = isRML ? "ResoniteModLoader" : e.current.files[0].name;
    const result = checks[e.id]?.fingerprint === fingerprint(e) ? checks[e.id] : undefined;
    const c = result?.value;
    return <Stack gap="xs" key={e.id} py="sm">
      <Group justify="space-between" align="center" gap="sm">
        <Box style={{ minWidth: 0, flex: "1 1 220px", overflowWrap: "anywhere" }}>
          <Text size="sm" fw={600}>{name}</Text>
          <Text size="xs" c="dimmed">{e.current.source?.repo ?? t("mods.uploaded")} · {e.current.source?.tag ?? new Date(e.current.addedAt).toLocaleString(i18n.language)}</Text>
        </Box>
        <Group gap="xs">
          <Switch label={t("mods.enabled")} aria-label={t("mods.enableName", { name })} checked={e.enabled} disabled={!editable} onChange={(event) => { const enabled = event.currentTarget.checked; void runChange(() => api.setModEnabled(e.id, enabled)); }} />
          {(isRML || e.updateSource) && <InspectorButton disabled={action.busy} onClick={() => check(e)}>{t("mods.check")}</InspectorButton>}
          {c && (c.updateAvailable || c.versionUnknown) && <InspectorButton disabled={!editable} onClick={() => update(e, c)}>{t("mods.updateTo", { tag: c.tag })}</InspectorButton>}
          <Menu position="bottom-end" withinPortal>
            <Menu.Target><ActionIcon aria-label={t("mods.actionsName", { name })} disabled={!editable}>⋮</ActionIcon></Menu.Target>
            <Menu.Dropdown>
              {!isRML && <Menu.Item onClick={() => setDialog({ kind: "upload", entry: e })}>{t("mods.replace")}</Menu.Item>}
              {!isRML && <Menu.Item onClick={() => setDialog({ kind: "source", entry: e })}>{t("mods.source")}</Menu.Item>}
              <Menu.Item disabled={!e.previous} onClick={() => void runChange(() => api.restoreMod(e.id))}>{t("mods.restore")}</Menu.Item>
              {!isRML && <Menu.Item color="red" onClick={() => confirm.ask({ title: t("mods.delete"), message: t("mods.deleteConfirm", { name }), danger: true, onConfirm: () => change(() => api.deleteMod(e.id)) })}>{t("mods.delete")}</Menu.Item>}
            </Menu.Dropdown>
          </Menu>
        </Group>
      </Group>
      {result?.failed && <Text size="xs" c="red" role="status">{t("mods.checkFailed")}</Text>}
      {c && <Text size="xs" c="dimmed" role="status">{c.versionUnknown ? t("mods.versionUnknown", { tag: c.tag }) : c.updateAvailable ? t("mods.available", { tag: c.tag }) : t("mods.upToDate")}</Text>}
    </Stack>;
  }

  return <ScrollArea h="100%" type="hover"><Stack maw={1000} mx="auto" pb="md" gap="lg">
    {error !== null && <Alert color="red"><Group justify="space-between"><Text size="sm">{t("mods.loadFailed")}{error && `: ${error}`}</Text><Button onClick={() => void load()}>{t("mods.retry")}</Button></Group></Alert>}
    {!info && error === null && <Loader size="sm" />}
    {info && <>
      {(status?.state !== "stopped" || !info.canModify) && <Text size="sm" c="dimmed">{info.busy ? t("mods.busy") : t("mods.stopRequired")}</Text>}
      <InspectorCard title="ResoniteModLoader">
        {rml ? row(rml) : <Group justify="space-between" py="sm"><Text size="sm" c="dimmed">{t("mods.notInstalled")}</Text><InspectorButton disabled={!editable} loading={action.busy} onClick={installRML}>{t("mods.install")}</InspectorButton></Group>}
        <Text size="xs" c="dimmed">{t("mods.harmony")}</Text>
      </InspectorCard>
      <InspectorCard title={t("mods.list")}>
        <Group justify="flex-end" py="sm" gap="xs"><InspectorButton disabled={!editable} onClick={() => setDialog({ kind: "upload" })}>{t("mods.upload")}</InspectorButton><InspectorButton disabled={!editable} onClick={() => setDialog({ kind: "github" })}>{t("mods.github")}</InspectorButton></Group>
        {info.entries.filter((e) => e.id !== "rml").map((e) => <Box key={e.id}><Divider />{row(e)}</Box>)}
        {!info.entries.some((e) => e.id !== "rml") && <Text c="dimmed" size="sm" py="md">{t("mods.empty")}</Text>}
      </InspectorCard>
    </>}
    {dialog && <AddModDialog dialog={dialog} editable={editable} busy={action.busy} onClose={() => { if (!action.busy) setDialog(null); }} onSave={(fn) => void runChange(fn)} />}
    <ConfirmHost confirm={confirm} />
  </Stack></ScrollArea>;
}

function AddModDialog({ dialog, editable, busy, onClose, onSave }: {
  dialog: Dialog; editable: boolean; busy: boolean; onClose: () => void; onSave: (fn: () => Promise<api.WriteResult>) => void;
}) {
  const { t } = useTranslation();
  const [file, setFile] = useState<File | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const dragDepth = useRef(0);
  const [dragging, setDragging] = useState(false);
  const [fileError, setFileError] = useState(false);
  const [repo, setRepo] = useState(dialog.check?.repo ?? dialog.entry?.updateSource?.repo ?? "");
  const [releases, setReleases] = useState<api.ModRelease[]>([]);
  const [tag, setTag] = useState<string | null>(null);
  const [asset, setAsset] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const [more, setMore] = useState(false);
  const [failed, setFailed] = useState(false);
  const [loading, setLoading] = useState(false);
  const release = releases.find((r) => r.tag_name === tag);
  const upload = dialog.kind === "upload";
  const title = upload ? dialog.entry ? t("mods.replace") : t("mods.upload") : dialog.kind === "source" ? t("mods.source") : t("mods.github");

  async function fetchReleases(nextPage = 1) {
    setLoading(true); setFailed(false);
    const r = await api.getModReleases(repo, nextPage);
    if (r.ok) {
      const data = r.data as api.ModReleases;
      setReleases((prev) => nextPage === 1 ? data.releases : [...prev, ...data.releases.filter((n) => !prev.some((p) => p.tag_name === n.tag_name))]);
      setPage(nextPage); setMore(data.hasMore);
      if (nextPage === 1) {
        const selected = data.releases.find((v) => v.tag_name === dialog.check?.tag) ?? data.releases[0];
        setTag(selected?.tag_name ?? null);
        const preferred = dialog.check?.asset || dialog.entry?.updateSource?.asset;
        setAsset(selected?.assets.find((a) => a.name === preferred)?.name ?? (selected?.assets.length === 1 ? selected.assets[0].name : null));
      }
    } else { setFailed(true); }
    setLoading(false);
  }

  function selectFile(files: File[]) {
    if (!editable || busy) return;
    const selected = files.length === 1 ? files[0] : null;
    const valid = selected !== null && selected.size > 0 && selected.size <= 32 * 1024 * 1024 && selected.name.toLowerCase().endsWith(".dll");
    setFile(valid ? selected : null);
    setFileError(!valid);
  }

  function save() {
    if (!editable || busy) return;
    if (upload && file) {
      onSave(() => api.uploadMod(file, dialog.entry?.id));
    } else if (tag && asset) {
      const source = { repo, tag, asset };
      onSave(() => dialog.kind === "source" && dialog.entry ? api.setModSource(dialog.entry.id, source) : api.downloadMod({ ...source, id: dialog.entry?.id }));
    }
  }

  return <Modal opened onClose={onClose} title={title} centered closeOnClickOutside={!busy} closeOnEscape={!busy} withCloseButton={!busy}>
    <Stack onDragOver={(event) => { if (upload) event.preventDefault(); }} onDrop={(event) => { if (upload) event.preventDefault(); }}>
      {dialog.entry && <Text size="sm">{dialog.entry.current.files[0].name}</Text>}
      {upload ? <>
        <Stack role="region" aria-label={t("mods.file")} aria-disabled={!editable} align="center" gap="xs" p="xl"
          style={{ border: `2px dashed var(${dragging && editable ? "--mantine-primary-color-filled" : "--mantine-color-dark-3"})`, borderRadius: "var(--mantine-radius-md)", background: dragging && editable ? "var(--mantine-primary-color-light)" : undefined, opacity: editable ? 1 : 0.5 }}
          onDragEnter={(event) => {
            event.preventDefault();
            if (editable && event.dataTransfer.types.includes("Files")) { dragDepth.current++; setDragging(true); }
          }}
          onDragOver={(event) => { event.preventDefault(); event.dataTransfer.dropEffect = editable ? "copy" : "none"; }}
          onDragLeave={(event) => { event.preventDefault(); dragDepth.current = Math.max(0, dragDepth.current - 1); if (!dragDepth.current) setDragging(false); }}
          onDrop={(event) => { event.preventDefault(); event.stopPropagation(); dragDepth.current = 0; setDragging(false); selectFile(Array.from(event.dataTransfer.files)); }}>
          <Text size="sm" ta="center">{t(dragging && editable ? "mods.dropHere" : "mods.dropDLL")}</Text>
          <Text size="xs" c="dimmed">{t("mods.fileLimit")}</Text>
          <input ref={fileInput} type="file" accept=".dll" hidden disabled={!editable} aria-label={t("mods.file")} onChange={(event) => {
            if (event.currentTarget.files?.length) selectFile(Array.from(event.currentTarget.files));
            event.currentTarget.value = "";
          }} />
          <Button disabled={!editable} onClick={() => fileInput.current?.click()}>{t(file ? "mods.chooseAgain" : "mods.chooseFile")}</Button>
        </Stack>
        <Text size="sm" aria-live="polite" style={{ overflowWrap: "anywhere" }}>{file ? `${file.name} (${(file.size / 1024 / 1024).toFixed(2)} MiB)` : ""}</Text>
        {fileError && <Text size="sm" c="red" role="alert">{t("mods.invalidFile")}</Text>}
      </> : <>
        <TextInput label={t("mods.repo")} placeholder="https://github.com/owner/repo" value={repo} disabled={busy || loading} onChange={(e) => { setRepo(e.currentTarget.value); setReleases([]); setTag(null); setAsset(null); setMore(false); }} />
        <Button onClick={() => void fetchReleases()} loading={loading} disabled={!repo.trim() || busy}>{t("mods.fetchReleases")}</Button>
        {failed && <Text c="red" size="sm" role="alert">{t("mods.releasesFailed")}</Text>}
        {page > 0 && !loading && releases.length === 0 && <Text size="sm" c="dimmed">{t("mods.noReleases")}</Text>}
        <Select label={t("mods.release")} data={releases.map((r) => ({ value: r.tag_name, label: r.tag_name }))} value={tag} disabled={busy || loading} onChange={(value) => { setTag(value); const assets = releases.find((r) => r.tag_name === value)?.assets ?? []; setAsset(assets.length === 1 ? assets[0].name : null); }} />
        {more && <Button size="xs" disabled={busy || loading} onClick={() => void fetchReleases(page + 1)}>{t("mods.older")}</Button>}
        <Select label={t("mods.file")} data={release?.assets.map((a) => a.name) ?? []} value={asset} disabled={busy || loading} onChange={setAsset} />
        {release && release.assets.length === 0 && <Text size="sm" c="dimmed">{t("mods.noAssets")}</Text>}
      </>}
      <Group justify="flex-end"><Button onClick={onClose} disabled={busy}>{t("mods.cancel")}</Button><Button loading={busy} disabled={!editable || loading || (upload ? !file : !tag || !asset)} onClick={save}>{dialog.kind === "source" ? t("mods.saveSource") : dialog.entry ? t("mods.apply") : t("mods.add")}</Button></Group>
    </Stack>
  </Modal>;
}
