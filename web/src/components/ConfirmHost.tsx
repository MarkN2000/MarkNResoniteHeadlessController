import { ConfirmModal } from "./ConfirmModal";
import { useConfirm } from "../hooks/useConfirm";

// useConfirm() の戻り値を渡すだけで確認モーダルを描画する定型ホスト。
// 各タブで繰り返していた <ConfirmModal opened=.../> の 7 行を 1 行に集約する（useConfirm とペアで使う）。
export function ConfirmHost({ confirm }: { confirm: ReturnType<typeof useConfirm> }) {
  return (
    <ConfirmModal
      opened={confirm.request !== null}
      title={confirm.request?.title ?? ""}
      message={confirm.request?.message}
      danger={confirm.request?.danger}
      loading={confirm.busy}
      onConfirm={() => void confirm.confirm()}
      onClose={confirm.close}
    />
  );
}
