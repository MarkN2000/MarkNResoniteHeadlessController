import { useTranslation } from "react-i18next";
import { Button, Group, Modal, Text } from "@mantine/core";

// 操作前の共通確認モーダル（アプリ全体で再利用）。
// danger=true で確定ボタンを赤に。@mantine/modals は未導入のため core の Modal を使う。
export function ConfirmModal({
  opened,
  title,
  message,
  danger,
  loading,
  onConfirm,
  onClose,
}: {
  opened: boolean;
  title: string;
  message?: string;
  danger?: boolean;
  loading?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Modal opened={opened} onClose={onClose} title={title} centered>
      {message && (
        <Text size="sm" mb="md">
          {message}
        </Text>
      )}
      <Group justify="flex-end" gap="xs">
        <Button variant="default" onClick={onClose}>
          {t("common.cancel")}
        </Button>
        {/* 危険ボタンは filled red。theme の autoContrast だと濃色文字になるため、
            白文字を保つよう autoContrast={false} で opt-out（default variant 時は無影響）。 */}
        <Button
          color={danger ? "red" : undefined}
          variant={danger ? "filled" : "default"}
          autoContrast={false}
          loading={loading}
          onClick={onConfirm}
        >
          {t("common.confirm")}
        </Button>
      </Group>
    </Modal>
  );
}
