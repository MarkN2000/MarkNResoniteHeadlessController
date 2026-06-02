import { useState } from "react";
import { ActionIcon, Button, Divider, Group, Stack, Switch, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import {
  CollapsibleSection,
  FieldRow,
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
  asNumOr,
  asStr,
  csvToArray,
  defaultWorld,
  getWorlds,
  removeWorld,
  setWorld,
} from "./configModel";
import { BufferedTextInput, CustomSessionIdInput } from "./fields";

// -1=無効 型フィールドの既定値（sample/default.json のスキーマ値・R6）。
// 未設定なら既定を表示し、空欄にされたら -1（無効）へスナップして「必ず数値」を保つ。
const SENTINEL_DEFAULTS: Record<string, number> = {
  awayKickMinutes: -1,
  idleRestartInterval: 1800,
  forcedRestartInterval: -1,
  autosaveInterval: -1,
};

// startWorlds[] のタブ式エディタ。タブ=1ワールド、＋で追加、最後の1枚は削除不可。
export function WorldsSection({
  cfg,
  onChange,
  centralUserId,
}: {
  cfg: ConfigMap;
  onChange: (cfg: ConfigMap) => void;
  centralUserId?: string; // customSessionId prefix の自動入力元（R12）
}) {
  const { t } = useTranslation();
  const worlds = getWorlds(cfg);
  const [active, setActive] = useState(0);
  const confirm = useConfirm();
  const idx = Math.min(active, worlds.length - 1); // 削除でズレたとき安全に丸める
  const world: WorldMap = worlds[idx] ?? {};

  const setW = (key: string, value: unknown) => onChange(setWorld(cfg, idx, { ...world, [key]: value }));
  // 数値フィールドの onChange ファクトリ。空欄のとき map に書く値だけが用途で異なる:
  //   numW→""（既存）／ sentinelW→-1（-1=無効・空欄を "" にせず必ず数値・R6）／
  //   portW→undefined（保存JSONから省く＝null/自動・R13）。
  const numWith = (empty: unknown) => (key: string) => (v: number | string) => setW(key, v === "" ? empty : Number(v));
  const numW = numWith("");
  const sentinelW = numWith(-1);
  const portW = numWith(undefined);

  // マーカークリック＝そのワールド項目を defaultWorld() の既定値へ戻す（確認あり）。
  // 雛形に無いキーは undefined＝暗黙の既定（空/フォールバック）に戻る。
  const resetProps = (key: string, fieldLabel: string) => ({
    markerLabel: t("common.resetToDefault"),
    onMarkerClick: () =>
      confirm.ask({
        title: t("common.resetConfirmTitle"),
        message: t("common.resetConfirmMsg", { field: fieldLabel }),
        onConfirm: () => setW(key, defaultWorld()[key]),
      }),
  });

  const onAdd = () => {
    onChange(addWorld(cfg));
    setActive(worlds.length);
  };
  // i 番目のワールドを削除（R5: 各タブの×から呼ぶ・index 引数化）。
  const askRemove = (i: number) =>
    confirm.ask({
      title: t("config.removeWorld"),
      message: t("config.confirmRemoveWorld", { name: asStr(worlds[i]?.sessionName) || `#${i + 1}` }),
      danger: true,
      onConfirm: () => {
        onChange(removeWorld(cfg, i));
        // 削除位置が active より前なら active を 1 つ詰め、末尾削除に備えて新範囲へクランプ。
        setActive((a) => Math.max(0, Math.min(i < a ? a - 1 : a, worlds.length - 2)));
      },
    });

  const worldLabel = (w: WorldMap, i: number) => {
    const name = asStr(w.sessionName) || `#${i + 1}`;
    return asBool(w.isEnabled, true) ? name : `${name} (${t("config.disabled")})`;
  };

  return (
    <Stack gap={6}>
      {/* ワールドタブ。各タブ＝選択ボタン＋×（R5・ConfigList と同方式でネストボタンを避ける）。
          最後の1枚は × 非表示（唯一のワールドは削除不可）。 */}
      <Group gap={4} wrap="wrap">
        {worlds.map((w, i) => (
          <Group key={i} gap={2} wrap="nowrap">
            <Button
              size="xs"
              variant={i === idx ? "filled" : "default"}
              color="gray"
              onClick={() => setActive(i)}
            >
              {worldLabel(w, i)}
            </Button>
            {worlds.length > 1 && (
              <ActionIcon
                size="sm"
                variant="subtle"
                color="red"
                aria-label={t("config.removeWorld")}
                title={t("config.removeWorld")}
                onClick={() => askRemove(i)}
              >
                ×
              </ActionIcon>
            )}
          </Group>
        ))}
        <Button size="xs" variant="light" color="gray" onClick={onAdd} aria-label={t("config.addWorld")}>
          ＋
        </Button>
      </Group>

      {worlds.length > 0 && (
        <>
          <FieldRow label={t("config.isEnabled")} {...resetProps("isEnabled", t("config.isEnabled"))}>
            <Switch checked={asBool(world.isEnabled, true)} onChange={(e) => setW("isEnabled", e.currentTarget.checked)} />
          </FieldRow>
          <FieldRow label={t("config.sessionName")} {...resetProps("sessionName", t("config.sessionName"))}>
            <InspectorTextInput value={asStr(world.sessionName)} onChange={(e) => setW("sessionName", e.currentTarget.value)} />
          </FieldRow>
          <FieldRow label={t("session.description")} align="start" {...resetProps("description", t("session.description"))}>
            <InspectorTextarea value={asStr(world.description)} onChange={(e) => setW("description", e.currentTarget.value)} />
          </FieldRow>
          <FieldRow label={t("session.accessLevel")} {...resetProps("accessLevel", t("session.accessLevel"))}>
            <InspectorSelect
              data={[...api.ACCESS_LEVELS]}
              value={asStr(world.accessLevel) || "Private"}
              onChange={(v) => v && setW("accessLevel", v)}
            />
          </FieldRow>
          <FieldRow label={t("session.maxUsers")} {...resetProps("maxUsers", t("session.maxUsers"))}>
            <InspectorNumberInput value={asNum(world.maxUsers)} onChange={numW("maxUsers")} min={1} allowNegative={false} />
          </FieldRow>
          <FieldRow label={t("config.loadWorldPresetName")} {...resetProps("loadWorldPresetName", t("config.loadWorldPresetName"))}>
            <InspectorSelect
              data={[...api.WORLD_TEMPLATES]}
              value={asStr(world.loadWorldPresetName) || "Grid"}
              onChange={(v) => v && setW("loadWorldPresetName", v)}
            />
          </FieldRow>
          <FieldRow label={t("config.loadWorldURL")} {...resetProps("loadWorldURL", t("config.loadWorldURL"))}>
            <InspectorTextInput
              value={asStr(world.loadWorldURL)}
              onChange={(e) => setW("loadWorldURL", e.currentTarget.value)}
              placeholder="resrec://..."
            />
          </FieldRow>
          {/* customSessionId はバッファ付き入力（内部 state）でリセットが表示へ反映されないため対象外。 */}
          <FieldRow label={t("config.customSessionId")} align="start">
            {/* key に centralUserId を含め、UserID が後着でも prefix 自動入力が反映されるよう再シード。
                commit 済の prefix/suffix は initial（map 値）から復元される。ただし UserID 後着の瞬間に
                「意図的に空にした prefix」は autoPrefix で再シードされ自動入力が復活し得る（後着は通常1回・
                getCredentials の ~100ms 窓のみで実害は軽微）。 */}
            <CustomSessionIdInput
              key={`${idx}:${centralUserId ?? ""}`}
              initial={asStr(world.customSessionId)}
              autoPrefix={centralUserId}
              onChange={(v) => setW("customSessionId", v)}
            />
          </FieldRow>

          <Divider my={4} color="dark.4" />
          {/* 運用項目群は折りたたみ（既定=閉じ）。基本項目だけ常時表示しスマホの縦長を抑える（R11）。 */}
          <CollapsibleSection title={t("config.operationSection")}>
            <Stack gap={6}>
              <Text size="xs" c="dimmed">
                {t("config.sentinelNote")}
              </Text>
              {/* tags はバッファ付き入力（内部 state）でリセットが表示へ反映されないため対象外。 */}
              <FieldRow label={t("config.tags")}>
                <BufferedTextInput
                  key={idx}
                  initial={arrayToCsv(world.tags)}
                  parse={csvToArray}
                  onCommit={(v) => setW("tags", v)}
                  placeholder={t("config.csvPlaceholder")}
                />
              </FieldRow>
              {/* -1=無効 型（R6）: 未設定なら既定値を表示し、空欄は -1 へスナップ＝常に数値。 */}
              <FieldRow label={t("config.awayKickMinutes")} {...resetProps("awayKickMinutes", t("config.awayKickMinutes"))}>
                <InspectorNumberInput
                  value={asNumOr(world.awayKickMinutes, SENTINEL_DEFAULTS.awayKickMinutes)}
                  onChange={sentinelW("awayKickMinutes")}
                />
              </FieldRow>
              <FieldRow label={t("config.idleRestartInterval")} {...resetProps("idleRestartInterval", t("config.idleRestartInterval"))}>
                <InspectorNumberInput
                  value={asNumOr(world.idleRestartInterval, SENTINEL_DEFAULTS.idleRestartInterval)}
                  onChange={sentinelW("idleRestartInterval")}
                />
              </FieldRow>
              <FieldRow label={t("config.forcedRestartInterval")} {...resetProps("forcedRestartInterval", t("config.forcedRestartInterval"))}>
                <InspectorNumberInput
                  value={asNumOr(world.forcedRestartInterval, SENTINEL_DEFAULTS.forcedRestartInterval)}
                  onChange={sentinelW("forcedRestartInterval")}
                />
              </FieldRow>
              <FieldRow label={t("config.autosaveInterval")} {...resetProps("autosaveInterval", t("config.autosaveInterval"))}>
                <InspectorNumberInput
                  value={asNumOr(world.autosaveInterval, SENTINEL_DEFAULTS.autosaveInterval)}
                  onChange={sentinelW("autosaveInterval")}
                />
              </FieldRow>
              <FieldRow label={t("config.saveOnExit")} {...resetProps("saveOnExit", t("config.saveOnExit"))}>
                <Switch checked={asBool(world.saveOnExit)} onChange={(e) => setW("saveOnExit", e.currentTarget.checked)} />
              </FieldRow>
              <FieldRow label={t("config.autoRecover")} {...resetProps("autoRecover", t("config.autoRecover"))}>
                <Switch checked={asBool(world.autoRecover, true)} onChange={(e) => setW("autoRecover", e.currentTarget.checked)} />
              </FieldRow>
              <FieldRow label={t("config.autoSleep")} {...resetProps("autoSleep", t("config.autoSleep"))}>
                <Switch checked={asBool(world.autoSleep, true)} onChange={(e) => setW("autoSleep", e.currentTarget.checked)} />
              </FieldRow>
              <FieldRow label={t("config.hideFromPublicListing")} {...resetProps("hideFromPublicListing", t("config.hideFromPublicListing"))}>
                <Switch
                  checked={asBool(world.hideFromPublicListing)}
                  onChange={(e) => setW("hideFromPublicListing", e.currentTarget.checked)}
                />
              </FieldRow>
              <FieldRow label={t("config.mobileFriendly")} {...resetProps("mobileFriendly", t("config.mobileFriendly"))}>
                <Switch checked={asBool(world.mobileFriendly)} onChange={(e) => setW("mobileFriendly", e.currentTarget.checked)} />
              </FieldRow>
              {/* ResoniteLink（R13）。port は空＝自動（未設定）＝保存JSONから省く。 */}
              <FieldRow label={t("config.enableResoniteLink")} {...resetProps("enableResoniteLink", t("config.enableResoniteLink"))}>
                <Switch
                  checked={asBool(world.enableResoniteLink)}
                  onChange={(e) => setW("enableResoniteLink", e.currentTarget.checked)}
                />
              </FieldRow>
              <FieldRow label={t("config.forceResoniteLinkPort")} {...resetProps("forceResoniteLinkPort", t("config.forceResoniteLinkPort"))}>
                <InspectorNumberInput
                  value={asNum(world.forceResoniteLinkPort)}
                  onChange={portW("forceResoniteLinkPort")}
                  min={1}
                  max={65535}
                  allowNegative={false}
                  placeholder={t("config.resoniteLinkPortHint")}
                />
              </FieldRow>
            </Stack>
          </CollapsibleSection>
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
