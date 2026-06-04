import { Button, Group, Stack, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import type { ConfigSummary } from "../../api";
import { InspectorButton, InspectorCard, RowIconButton } from "../../components/inspector";

// config 一覧（master）。各行 = 名前（クリックで編集）＋ 複製/削除。worldCount は出さない（名前のみ）。
// 右パネル（ConfigEditor）と同じ InspectorCard で見た目を揃え、SplitColumns に並べる。
export function ConfigList({
  list,
  selected,
  onSelect,
  onNew,
  onDuplicate,
  onDelete,
}: {
  list: ConfigSummary[];
  selected: string | null;
  onSelect: (name: string) => void;
  onNew: () => void;
  onDuplicate: (name: string) => void;
  onDelete: (name: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <InspectorCard
      title={t("config.listTitle")}
      actions={
        <InspectorButton severity="neutral" onClick={onNew}>
          ＋ {t("config.new")}
        </InspectorButton>
      }
    >
      <Stack gap={4}>
        {list.map((c) => {
          const active = c.name === selected;
          return (
            <Group key={c.name} gap={4} wrap="nowrap">
              <Button
                size="xs"
                variant={active ? "filled" : "default"}
                color="gray"
                justify="flex-start"
                onClick={() => onSelect(c.name)}
                style={{ flex: 1, minWidth: 0 }}
                styles={{
                  label: {
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                  },
                }}
              >
                {c.name}
              </Button>
              <RowIconButton color="green" label={t("config.duplicate")} onClick={() => onDuplicate(c.name)}>
                ⧉
              </RowIconButton>
              <RowIconButton color="red" label={t("config.delete")} onClick={() => onDelete(c.name)}>
                ×
              </RowIconButton>
            </Group>
          );
        })}
        {list.length === 0 && (
          <Text size="xs" c="dimmed" ta="center" mt="xs">
            {t("config.empty")}
          </Text>
        )}
      </Stack>
    </InspectorCard>
  );
}
