import { useState } from "react";
import { Button, Divider, Group, Stack, Switch, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import {
  CollapsibleSection,
  FieldRow,
  InspectorNumberInput,
  InspectorSelect,
  InspectorTextInput,
  InspectorTextarea,
  RowIconButton,
  SelectionButton,
} from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import type { ConfigMap, WorldMap } from "./configModel";
import { WorldUrlSearch } from "./WorldUrlSearch";
import {
  addWorld,
  arrayToCsv,
  asBool,
  asNum,
  asNumOr,
  asStr,
  csvToArray,
  defaultWorld,
  getDuplicateUDPPorts,
  getForcePort,
  getStringArray,
  getWorlds,
  hasLegacyForcePort,
  removeWorld,
  setForcePort,
  setWorld,
} from "./configModel";
import type { ForcePortProtocol } from "./configModel";
import { BufferedTextInput, CustomSessionIdInput, RolePairsInput, SentinelNumberInput, StringListInput } from "./fields";
import { AdvancedFieldsEditor } from "./AdvancedFieldsEditor";
import { WORLD_DEDICATED_KEYS, WORLD_NICHE_CATALOG } from "./fieldCatalog";

// -1=無効 型フィールドの既定値（sample/default.json のスキーマ値・R6）。
// 未設定なら既定を表示し、空欄にされたら -1（無効）へスナップして「必ず数値」を保つ。
// 注: defaultWorld()（configModel.ts）が現在これらのキーを明示的に持つため、新規 config では
// 常にキーが存在しこのフォールバックは不発火。発火するのは旧（キー欠落）コンフィグの表示用のみ
// ＝旧データは headless 既定で動いていたため当時の値（awayKick=-1 等）で表示する。
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
  const duplicateUDPPorts = getDuplicateUDPPorts(cfg);

  const setW = (key: string, value: unknown) => onChange(setWorld(cfg, idx, { ...world, [key]: value }));
  // 数値フィールドの onChange ファクトリ。空欄のとき map に書く値だけが異なる:
  //   omitW→undefined（保存JSONからキーを省く＝headless 既定/自動。maxUsers・port）／
  //   sentinelW→-1（-1=無効。空欄でも必ず数値を保つ・R6）。
  // 空欄を "" で書くと保存JSONに文字列が混入し、headless が数値型を期待する箇所で不整合になるため、
  // 数値欄は undefined（キー省略）か -1（無効）のどちらかに必ず正規化する（M1）。
  const numWith = (empty: unknown) => (key: string) => (v: number | string) => setW(key, v === "" ? empty : Number(v));
  const omitW = numWith(undefined);
  const sentinelW = numWith(-1);
  // テキスト欄: 空文字（空白のみ含む）は null（未設定）として保存し「空欄を登録しない」。
  // 未設定なら Resonite 既定が使われる（例: sessionName 空→ワールド名）。対象は JSON Schema 上
  // いずれも null 許容（type に "null" を含む）。配列欄（tags/autoSpawnItems）は各 onCommit で空→null。
  const setWText = (key: string, v: string) => setW(key, v.trim() === "" ? null : v);

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

  const setProtocolPort = (protocol: ForcePortProtocol, value: number | string) =>
    onChange(setWorld(cfg, idx, setForcePort(world, protocol, value)));
  const resetProtocolProps = (protocol: ForcePortProtocol, fieldLabel: string) => ({
    markerLabel: t("common.resetToDefault"),
    onMarkerClick: () =>
      confirm.ask({
        title: t("common.resetConfirmTitle"),
        message: t("common.resetConfirmMsg", { field: fieldLabel }),
        onConfirm: () => setProtocolPort(protocol, ""),
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
            <SelectionButton selected={i === idx} onClick={() => setActive(i)}>
              {worldLabel(w, i)}
            </SelectionButton>
            {worlds.length > 1 && (
              <RowIconButton color="red" label={t("config.removeWorld")} onClick={() => askRemove(i)}>
                ×
              </RowIconButton>
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
            <InspectorTextInput
              value={asStr(world.sessionName)}
              onChange={(e) => setWText("sessionName", e.currentTarget.value)}
              placeholder={t("config.sessionNameHint")}
            />
          </FieldRow>
          <FieldRow label={t("session.description")} align="start" {...resetProps("description", t("session.description"))}>
            <InspectorTextarea value={asStr(world.description)} onChange={(e) => setWText("description", e.currentTarget.value)} />
          </FieldRow>
          {/* 新規既定は defaultWorld() の accessLevel=Anyone。下の || "Private" は旧（キー欠落）
              コンフィグの表示用フォールバックで、新規では不発火（既定値は雛形が単一の真実）。 */}
          <FieldRow label={t("session.accessLevel")} {...resetProps("accessLevel", t("session.accessLevel"))}>
            <InspectorSelect
              data={[...api.ACCESS_LEVELS]}
              value={asStr(world.accessLevel) || "Private"}
              onChange={(v) => v && setW("accessLevel", v)}
            />
          </FieldRow>
          <FieldRow label={t("session.maxUsers")} {...resetProps("maxUsers", t("session.maxUsers"))}>
            <InspectorNumberInput value={asNum(world.maxUsers)} onChange={omitW("maxUsers")} min={1} allowNegative={false} />
          </FieldRow>
          <FieldRow label={t("config.loadWorldPresetName")} {...resetProps("loadWorldPresetName", t("config.loadWorldPresetName"))}>
            <InspectorSelect
              data={[...api.WORLD_TEMPLATES]}
              value={asStr(world.loadWorldPresetName) || "Grid"}
              onChange={(v) => v && setW("loadWorldPresetName", v)}
            />
          </FieldRow>
          {/* URL欄＋「検索 ▾」トグル＋Collapse検索パネル（UI改善②・FieldRowごと所有）。
              「選択」でカードのURLをそのままセット（sessionName 等は触らない）。 */}
          <WorldUrlSearch
            label={t("config.loadWorldURL")}
            {...resetProps("loadWorldURL", t("config.loadWorldURL"))}
            value={asStr(world.loadWorldURL)}
            onChange={(v) => setWText("loadWorldURL", v)}
            onPickUrl={(url) => setW("loadWorldURL", url)}
          />
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
              onChange={(v) => setW("customSessionId", v || null)}
            />
          </FieldRow>

          {/* -1=無効 型（R6）の注記。センチネル欄は基本(awayKick/idleRestart)と上級(強制再起動/自動保存)に分かれるため両方に出す。 */}
          <Text size="xs" c="dimmed">
            {t("config.sentinelNote")}
          </Text>
          {/* tags はバッファ付き入力（内部 state）でリセットが表示へ反映されないため対象外。 */}
          <FieldRow label={t("config.tags")}>
            <BufferedTextInput
              key={idx}
              initial={arrayToCsv(world.tags)}
              parse={csvToArray}
              onCommit={(v) => setW("tags", Array.isArray(v) && v.length ? v : null)}
              placeholder={t("config.csvPlaceholder")}
            />
          </FieldRow>
          {/* -1=無効 型（R6）: 未設定なら既定値を表示し、空欄は -1 へスナップ＝常に数値。 */}
          <FieldRow label={t("config.awayKickMinutes")} {...resetProps("awayKickMinutes", t("config.awayKickMinutes"))}>
            <SentinelNumberInput
              value={asNumOr(world.awayKickMinutes, SENTINEL_DEFAULTS.awayKickMinutes)}
              onChange={sentinelW("awayKickMinutes")}
            />
          </FieldRow>
          <FieldRow label={t("config.idleRestartInterval")} {...resetProps("idleRestartInterval", t("config.idleRestartInterval"))}>
            <SentinelNumberInput
              value={asNumOr(world.idleRestartInterval, SENTINEL_DEFAULTS.idleRestartInterval)}
              onChange={sentinelW("idleRestartInterval")}
            />
          </FieldRow>
          {/* ①一般（続き）: ロール事前割当・自動招待。 */}
          <FieldRow label={t("config.defaultUserRoles")} align="start">
            {/* RolePairsInput は内部 state（buffered）のため key={idx} で再シード・リセット対象外（tags と同方針）。 */}
            <RolePairsInput
              key={idx}
              initial={world.defaultUserRoles}
              onChange={(v) => setW("defaultUserRoles", v)}
              userPlaceholder={t("config.userPlaceholder")}
              addLabel={t("config.add")}
            />
          </FieldRow>
          <FieldRow label={t("config.autoInviteUsernames")} align="start">
            <StringListInput
              key={idx}
              items={getStringArray(world.autoInviteUsernames)}
              onChange={(items) => setW("autoInviteUsernames", items.length ? items : null)}
              addLabel={t("config.add")}
              placeholder={t("config.userPlaceholder")}
            />
          </FieldRow>
          <FieldRow
            label={t("config.autoInviteMessage")}
            align="start"
            {...resetProps("autoInviteMessage", t("config.autoInviteMessage"))}
          >
            <InspectorTextarea
              value={asStr(world.autoInviteMessage)}
              onChange={(e) => setWText("autoInviteMessage", e.currentTarget.value)}
            />
          </FieldRow>

          <Divider my={4} color="dark.4" />
          {/* 上級設定（折りたたみ・既定閉じ）＝強制再起動/自動保存/終了時保存/自動復帰/モバイル対応
              ＋自動スリープ/一覧から隠す/ResoniteLink（有効+ポート）。後者4つは基本から移動。 */}
          <CollapsibleSection title={t("common.advancedSection")}>
            <Stack gap={6}>
              {/* -1=無効 型（R6）の注記（上級のセンチネル欄＝強制再起動/自動保存 用）。 */}
              <Text size="xs" c="dimmed">
                {t("config.sentinelNote")}
              </Text>
              <FieldRow label={t("config.forcedRestartInterval")} {...resetProps("forcedRestartInterval", t("config.forcedRestartInterval"))}>
                <SentinelNumberInput
                  value={asNumOr(world.forcedRestartInterval, SENTINEL_DEFAULTS.forcedRestartInterval)}
                  onChange={sentinelW("forcedRestartInterval")}
                />
              </FieldRow>
              <FieldRow label={t("config.autosaveInterval")} {...resetProps("autosaveInterval", t("config.autosaveInterval"))}>
                <SentinelNumberInput
                  value={asNumOr(world.autosaveInterval, SENTINEL_DEFAULTS.autosaveInterval)}
                  onChange={sentinelW("autosaveInterval")}
                />
              </FieldRow>
              <FieldRow label={t("config.saveOnExit")} {...resetProps("saveOnExit", t("config.saveOnExit"))}>
                <Switch checked={asBool(world.saveOnExit)} onChange={(e) => setW("saveOnExit", e.currentTarget.checked)} />
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
              {/* 招待リクエスト転送先（リスト追加式）。 */}
              <FieldRow label={t("config.inviteRequestHandlerUsernames")} align="start">
                <StringListInput
                  key={idx}
                  items={getStringArray(world.inviteRequestHandlerUsernames)}
                  onChange={(items) => setW("inviteRequestHandlerUsernames", items.length ? items : null)}
                  addLabel={t("config.add")}
                  placeholder={t("config.userPlaceholder")}
                />
              </FieldRow>
              {/* 保存者（未指定=null / LocalMachine / CloudUser）。"unset" は表示用センチネルで保存時 null。 */}
              <FieldRow label={t("config.saveAsOwner")} {...resetProps("saveAsOwner", t("config.saveAsOwner"))}>
                <InspectorSelect
                  data={[
                    { value: "unset", label: t("config.saveOwnerUnset") },
                    { value: "LocalMachine", label: t("config.saveOwnerLocal") },
                    { value: "CloudUser", label: t("config.saveOwnerCloud") },
                  ]}
                  value={asStr(world.saveAsOwner) || "unset"}
                  onChange={(v) => setW("saveAsOwner", !v || v === "unset" ? null : v)}
                />
              </FieldRow>
              {/* プロトコル別固定ポート。空欄のプロトコルは Resonite が範囲内からランダム選択する。
                  旧 forcePort は LNL の表示用フォールバックとし、いずれかを編集すると新形式へ移行する。 */}
              <Text size="xs" c="dimmed">
                {t("config.forcePortsHint")}
              </Text>
              {hasLegacyForcePort(world) && (
                <Text size="xs" c="yellow.7">
                  {t("config.forcePortLegacyNote")}
                </Text>
              )}
              <FieldRow
                label={t("config.forcePortLNL")}
                {...resetProtocolProps("lnl", t("config.forcePortLNL"))}
              >
                <InspectorNumberInput
                  value={getForcePort(world, "lnl")}
                  onChange={(value) => setProtocolPort("lnl", value)}
                  min={1}
                  max={65535}
                  allowNegative={false}
                  placeholder={t("config.forcePortAuto")}
                  error={
                    duplicateUDPPorts.has(Number(getForcePort(world, "lnl")))
                      ? t("config.udpPortDuplicate")
                      : undefined
                  }
                />
              </FieldRow>
              <FieldRow
                label={t("config.forcePortQUIC")}
                {...resetProtocolProps("quic", t("config.forcePortQUIC"))}
              >
                <InspectorNumberInput
                  value={getForcePort(world, "quic")}
                  onChange={(value) => setProtocolPort("quic", value)}
                  min={1}
                  max={65535}
                  allowNegative={false}
                  placeholder={t("config.forcePortAuto")}
                  error={
                    duplicateUDPPorts.has(Number(getForcePort(world, "quic")))
                      ? t("config.udpPortDuplicate")
                      : undefined
                  }
                />
              </FieldRow>
              <FieldRow
                label={t("config.forcePortTCP")}
                {...resetProtocolProps("tcp", t("config.forcePortTCP"))}
              >
                <InspectorNumberInput
                  value={getForcePort(world, "tcp")}
                  onChange={(value) => setProtocolPort("tcp", value)}
                  min={1}
                  max={65535}
                  allowNegative={false}
                  placeholder={t("config.forcePortAuto")}
                />
              </FieldRow>
              <Text size="xs" c="dimmed">
                {t("config.quicGlobalHint")}
              </Text>
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
                  onChange={omitW("forceResoniteLinkPort")}
                  min={1}
                  max={65535}
                  allowNegative={false}
                  placeholder={t("config.resoniteLinkPortHint")}
                />
              </FieldRow>
              <Divider my={4} color="dark.4" />
              {/* ③詳細フィールド（ワールド）: 専用フォームに無い公式キー（各クラウド変数/
                  overrideCorrespondingWorldId/mobileFriendly/autoRecover 等）を追加。内部 state を持つ子
                  （RawJsonInput）があるため key={idx} でワールド切替時に再マウント＝再シードする。 */}
              <AdvancedFieldsEditor
                key={idx}
                obj={world}
                onChange={(next) => onChange(setWorld(cfg, idx, next))}
                dedicated={WORLD_DEDICATED_KEYS}
                catalog={WORLD_NICHE_CATALOG}
              />
            </Stack>
          </CollapsibleSection>
        </>
      )}

      <ConfirmHost confirm={confirm} />
    </Stack>
  );
}
