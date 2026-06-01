import { useState } from "react";
import { Button, Divider, Group, Stack, Switch, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import {
  FieldRow,
  InspectorButton,
  InspectorNumberInput,
  InspectorSelect,
  InspectorTextInput,
  InspectorTextarea,
} from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useConfirm } from "../../hooks/useConfirm";
import type { ConfigMap, WorldMap } from "./configModel";
import {
  addWorld,
  arrayToCsv,
  asBool,
  asNum,
  asStr,
  csvToArray,
  getWorlds,
  removeWorld,
  setWorld,
} from "./configModel";
import { BufferedTextInput, CustomSessionIdInput } from "./fields";

// startWorlds[] のタブ式エディタ。タブ=1ワールド、＋で追加、最後の1枚は削除不可。
export function WorldsSection({ cfg, onChange }: { cfg: ConfigMap; onChange: (cfg: ConfigMap) => void }) {
  const { t } = useTranslation();
  const worlds = getWorlds(cfg);
  const [active, setActive] = useState(0);
  const confirm = useConfirm();
  const idx = Math.min(active, worlds.length - 1); // 削除でズレたとき安全に丸める
  const world: WorldMap = worlds[idx] ?? {};

  const setW = (key: string, value: unknown) => onChange(setWorld(cfg, idx, { ...world, [key]: value }));
  const numW = (key: string) => (v: number | string) => setW(key, v === "" ? "" : Number(v));

  const onAdd = () => {
    onChange(addWorld(cfg));
    setActive(worlds.length);
  };
  const askRemove = () =>
    confirm.ask({
      title: t("config.removeWorld"),
      message: t("config.confirmRemoveWorld", { name: asStr(world.sessionName) || `#${idx + 1}` }),
      danger: true,
      onConfirm: () => {
        onChange(removeWorld(cfg, idx));
        setActive(Math.max(0, idx - 1));
      },
    });

  const worldLabel = (w: WorldMap, i: number) => {
    const name = asStr(w.sessionName) || `#${i + 1}`;
    return asBool(w.isEnabled, true) ? name : `${name} (${t("config.disabled")})`;
  };

  return (
    <Stack gap={6}>
      <Group gap={4} wrap="wrap">
        {worlds.map((w, i) => (
          <Button
            key={i}
            size="xs"
            variant={i === idx ? "filled" : "default"}
            color="gray"
            onClick={() => setActive(i)}
            styles={i === idx ? { label: { color: "var(--mantine-color-dark-9)" } } : undefined}
          >
            {worldLabel(w, i)}
          </Button>
        ))}
        <Button size="xs" variant="light" color="gray" onClick={onAdd} aria-label={t("config.addWorld")}>
          ＋
        </Button>
      </Group>

      {worlds.length > 0 && (
        <>
          <FieldRow label={t("config.isEnabled")}>
            <Switch checked={asBool(world.isEnabled, true)} onChange={(e) => setW("isEnabled", e.currentTarget.checked)} />
          </FieldRow>
          <FieldRow label={t("config.sessionName")}>
            <InspectorTextInput value={asStr(world.sessionName)} onChange={(e) => setW("sessionName", e.currentTarget.value)} />
          </FieldRow>
          <FieldRow label={t("session.description")} align="start">
            <InspectorTextarea value={asStr(world.description)} onChange={(e) => setW("description", e.currentTarget.value)} />
          </FieldRow>
          <FieldRow label={t("session.accessLevel")}>
            <InspectorSelect
              data={[...api.ACCESS_LEVELS]}
              value={asStr(world.accessLevel) || "Private"}
              onChange={(v) => v && setW("accessLevel", v)}
            />
          </FieldRow>
          <FieldRow label={t("session.maxUsers")}>
            <InspectorNumberInput value={asNum(world.maxUsers)} onChange={numW("maxUsers")} min={1} allowNegative={false} />
          </FieldRow>
          <FieldRow label={t("config.loadWorldPresetName")}>
            <InspectorSelect
              data={[...api.WORLD_TEMPLATES]}
              value={asStr(world.loadWorldPresetName) || "Grid"}
              onChange={(v) => v && setW("loadWorldPresetName", v)}
            />
          </FieldRow>
          <FieldRow label={t("config.loadWorldURL")}>
            <InspectorTextInput
              value={asStr(world.loadWorldURL)}
              onChange={(e) => setW("loadWorldURL", e.currentTarget.value)}
              placeholder="resrec://..."
            />
          </FieldRow>
          <FieldRow label={t("config.customSessionId")} align="start">
            <CustomSessionIdInput key={idx} initial={asStr(world.customSessionId)} onChange={(v) => setW("customSessionId", v)} />
          </FieldRow>

          <Divider my={4} color="dark.4" label={t("config.operationSection")} labelPosition="center" />
          <Text size="xs" c="dimmed">
            {t("config.sentinelNote")}
          </Text>
          <FieldRow label={t("config.tags")}>
            <BufferedTextInput
              key={idx}
              initial={arrayToCsv(world.tags)}
              parse={csvToArray}
              onCommit={(v) => setW("tags", v)}
              placeholder={t("config.csvPlaceholder")}
            />
          </FieldRow>
          <FieldRow label={t("config.awayKickMinutes")}>
            <InspectorNumberInput value={asNum(world.awayKickMinutes)} onChange={numW("awayKickMinutes")} />
          </FieldRow>
          <FieldRow label={t("config.idleRestartInterval")}>
            <InspectorNumberInput value={asNum(world.idleRestartInterval)} onChange={numW("idleRestartInterval")} />
          </FieldRow>
          <FieldRow label={t("config.forcedRestartInterval")}>
            <InspectorNumberInput value={asNum(world.forcedRestartInterval)} onChange={numW("forcedRestartInterval")} />
          </FieldRow>
          <FieldRow label={t("config.autosaveInterval")}>
            <InspectorNumberInput value={asNum(world.autosaveInterval)} onChange={numW("autosaveInterval")} />
          </FieldRow>
          <FieldRow label={t("config.saveOnExit")}>
            <Switch checked={asBool(world.saveOnExit)} onChange={(e) => setW("saveOnExit", e.currentTarget.checked)} />
          </FieldRow>
          <FieldRow label={t("config.autoRecover")}>
            <Switch checked={asBool(world.autoRecover, true)} onChange={(e) => setW("autoRecover", e.currentTarget.checked)} />
          </FieldRow>
          <FieldRow label={t("config.autoSleep")}>
            <Switch checked={asBool(world.autoSleep, true)} onChange={(e) => setW("autoSleep", e.currentTarget.checked)} />
          </FieldRow>
          <FieldRow label={t("config.hideFromPublicListing")}>
            <Switch
              checked={asBool(world.hideFromPublicListing)}
              onChange={(e) => setW("hideFromPublicListing", e.currentTarget.checked)}
            />
          </FieldRow>
          <FieldRow label={t("config.mobileFriendly")}>
            <Switch checked={asBool(world.mobileFriendly)} onChange={(e) => setW("mobileFriendly", e.currentTarget.checked)} />
          </FieldRow>

          <InspectorButton severity="danger" disabled={worlds.length <= 1} onClick={askRemove} mt={4}>
            {t("config.removeWorld")}
          </InspectorButton>
        </>
      )}

      <ConfirmModal
        opened={confirm.request !== null}
        title={confirm.request?.title ?? ""}
        message={confirm.request?.message}
        danger={confirm.request?.danger}
        loading={confirm.busy}
        onConfirm={() => void confirm.confirm()}
        onClose={confirm.close}
      />
    </Stack>
  );
}
