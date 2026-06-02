import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import { useAsyncAction } from "../../hooks/useAsyncAction";

// 中央 Resonite アカウントのフォーム状態（読込/保存/保存可否）を管理する共通フック。
// 設定タブ（AccountSection）と初回モーダル（AccountSetupModal）で共用（UI 共通化・案A）。
//   - load(): GET /headless-credentials（password はマスクされるため空・hasPassword のみ反映）
//   - canSave: username 必須 + (新規 password 入力 or 既存 password あり)。
//     新規設定（pw 未登録）は password も必須／既存 pw ありなら username だけでも保存可（空=保持）。
//   - save(): PUT。成功で password 欄をクリアし hasPassword=true、onSaved を呼ぶ（バナー再評価等）。
export function useCredentialsForm(onSaved?: () => void) {
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [hasPassword, setHasPassword] = useState(false);
  const [userId, setUserId] = useState(""); // 解決済 UserID（表示用・R12）
  const [loaded, setLoaded] = useState(false);
  const apply = useAsyncAction();

  const load = useCallback(async () => {
    const c = await api.getCredentials();
    if (c) {
      setUsername(c.username);
      setHasPassword(c.hasPassword);
      setUserId(c.userId);
    }
    setPassword("");
    setLoaded(true);
  }, []);

  const canSave = username.trim() !== "" && (password !== "" || hasPassword);

  const save = () =>
    apply.run(async () => {
      const r = await api.putCredentials(username.trim(), password);
      if (r.ok) {
        setPassword("");
        setHasPassword(true);
        await load(); // 解決された UserID を表示へ反映（PUT 応答は WriteResult のため再取得）
        onSaved?.();
      }
      return r;
    }, t("settings.toastAccountSaved"));

  return { username, setUsername, password, setPassword, hasPassword, userId, loaded, load, canSave, busy: apply.busy, save };
}
