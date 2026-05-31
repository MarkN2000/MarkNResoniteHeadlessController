import { useState } from "react";
import { Stack, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import { FieldRow, InspectorCard, InspectorTextInput } from "../../components/inspector";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { SaveButton } from "./SaveButton";

// 管理パスワード変更（現PW + 新PW + 確認）。成功時 backend が新Cookieを再発行＝このブラウザは継続。
// 失敗（現PW誤り等）は WriteResult 経由で赤トースト（7-7 第1層）。一致/空チェックはクライアントで。
export function PasswordSection() {
  const { t } = useTranslation();
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [err, setErr] = useState<string | undefined>();
  const apply = useAsyncAction();

  const valid = cur !== "" && next !== "";

  const submit = () => {
    setErr(undefined);
    if (next === "") {
      setErr(t("settings.pwEmpty"));
      return;
    }
    if (next !== confirm) {
      setErr(t("settings.pwMismatch"));
      return;
    }
    void apply.run(async () => {
      const r = await api.changePassword(cur, next);
      if (r.ok) {
        setCur("");
        setNext("");
        setConfirm("");
      }
      return r;
    }, t("settings.toastPasswordChanged"));
  };

  return (
    <InspectorCard title={t("settings.passwordSection")}>
      <Stack gap={6}>
        <FieldRow label={t("settings.currentPassword")}>
          <InspectorTextInput type="password" value={cur} onChange={(e) => setCur(e.currentTarget.value)} />
        </FieldRow>
        <FieldRow label={t("settings.newPassword")}>
          <InspectorTextInput type="password" value={next} onChange={(e) => setNext(e.currentTarget.value)} />
        </FieldRow>
        <FieldRow label={t("settings.confirmPassword")}>
          <InspectorTextInput type="password" value={confirm} onChange={(e) => setConfirm(e.currentTarget.value)} />
        </FieldRow>
        {err && (
          <Text size="xs" c="red.6" ta="center">
            {err}
          </Text>
        )}
        <SaveButton label={t("settings.changePassword")} onClick={submit} disabled={!valid} loading={apply.busy} />
      </Stack>
    </InspectorCard>
  );
}
