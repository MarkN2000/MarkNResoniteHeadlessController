import { ActionIcon, Box, Center, Group, Loader, SimpleGrid, Stack, Text, UnstyledButton } from "@mantine/core";
import { Trans, useTranslation } from "react-i18next";
import type { World } from "../../api";
import { InspectorCard, RefreshButton } from "../../components/inspector";

interface Props {
  sessions: World[];
  focusedIdx: number;
  refreshing: boolean;
  onFocus: (idx: number) => void;
  onRefresh: () => void;
  onOpenNewSession: () => void;
}

function displayName(name: string): string {
  return name.replace(/<br\s*\/?>/gi, "\n");
}

export function SessionList({
  sessions,
  focusedIdx,
  refreshing,
  onFocus,
  onRefresh,
  onOpenNewSession,
}: Props) {
  const { t } = useTranslation();

  return (
    <InspectorCard
      title={t("session.listTitle", { count: sessions.length })}
      actions={
        <Group gap={6} wrap="nowrap" pr="sm">
          <RefreshButton onClick={onRefresh} loading={refreshing} label={t("session.refresh")} />
          <ActionIcon
            variant="light"
            color="green"
            size="lg"
            radius="md"
            onClick={onOpenNewSession}
            aria-label={t("session.openNew")}
            title={t("session.openNew")}
          >
            <Box
              component="span"
              aria-hidden="true"
              style={{
                width: 14,
                height: 14,
                display: "block",
                background:
                  "linear-gradient(var(--mantine-color-green-6), var(--mantine-color-green-6)) center / 14px 3px no-repeat, linear-gradient(var(--mantine-color-green-6), var(--mantine-color-green-6)) center / 3px 14px no-repeat",
              }}
            />
          </ActionIcon>
        </Group>
      }
    >
      {sessions.length === 0 ? (
        <Center mih={72}>
          {refreshing ? <Loader size="sm" /> : <Text c="dimmed">{t("session.noSessions")}</Text>}
        </Center>
      ) : (
        <SimpleGrid cols={{ base: 1, sm: 2, xl: 3 }} spacing="sm">
          {sessions.map((session) => {
            const focused = session.index === focusedIdx;
            return (
              <UnstyledButton
                key={session.index}
                type="button"
                aria-pressed={focused}
                onClick={() => onFocus(session.index)}
                style={{
                  width: "100%",
                  minHeight: 86,
                  padding: "var(--mantine-spacing-sm)",
                  border: `3px solid ${focused ? "var(--mantine-color-brand-6)" : "transparent"}`,
                  borderRadius: "var(--mantine-radius-md)",
                  backgroundColor: "var(--mantine-color-dark-6)",
                }}
              >
                <Stack gap={6}>
                  <Group gap="xs" wrap="nowrap" align="flex-start">
                    <Text size="sm" fw={700} c="brand.5" style={{ flexShrink: 0 }}>
                      #{session.index}
                    </Text>
                    <Text
                      size="sm"
                      fw={600}
                      lineClamp={2}
                      style={{ flex: 1, minWidth: 0, whiteSpace: "pre-line" }}
                    >
                      {displayName(session.name)}
                    </Text>
                    {focused && (
                      <Text size="xs" c="brand.5" style={{ flexShrink: 0, whiteSpace: "nowrap" }}>
                        {t("session.focused")}
                      </Text>
                    )}
                  </Group>
                  <Box>
                    <Group gap={0} align="baseline" wrap="wrap">
                      <Trans
                        i18nKey="session.listOccupancy"
                        values={{
                          present: session.present,
                          away: session.users - session.present,
                          maxUsers: session.maxUsers,
                          accessLevel: session.accessLevel,
                        }}
                        components={{
                          presentLabel: <Text component="span" size="xs" c="green.6" />,
                          presentCount: <Text component="span" size="sm" fw={600} />,
                          awayDetails: <Text component="span" size="xs" c="dimmed" />,
                          capacity: <Text component="span" size="sm" fw={600} />,
                        }}
                      />
                    </Group>
                  </Box>
                </Stack>
              </UnstyledButton>
            );
          })}
        </SimpleGrid>
      )}
    </InspectorCard>
  );
}
