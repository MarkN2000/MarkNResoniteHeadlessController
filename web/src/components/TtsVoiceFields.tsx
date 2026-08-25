import { useEffect } from "react";
import { Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import type { TtsSpeakersState } from "../hooks/useTtsSpeakers";
import { FieldRow, InspectorSelect, InspectorTextarea } from "./inspector";

// ttsVoice テンプレート用の共通入力。話者一覧の到着後、未選択（0/null）なら先頭を選ぶ。
export function TtsVoiceFields({
  text,
  speakerId,
  speakers,
  onTextChange,
  onSpeakerIdChange,
}: {
  text: string;
  speakerId: number | null;
  speakers: TtsSpeakersState;
  onTextChange: (value: string) => void;
  onSpeakerIdChange: (value: number) => void;
}) {
  const { t } = useTranslation();
  const { voices, loading, failed } = speakers;

  useEffect(() => {
    if (!loading && !failed && voices.length > 0 && (speakerId === null || speakerId === 0)) {
      onSpeakerIdChange(voices[0].id);
    }
  }, [failed, loading, onSpeakerIdChange, speakerId, voices]);

  const options = voices.map((voice) => ({
    value: String(voice.id),
    label: `${voice.speakerName} / ${voice.styleName}`,
  }));
  const placeholder = loading
    ? t("tts.speakersLoading")
    : failed
      ? t("tts.speakersLoadFailed")
      : t("tts.speakersEmpty");

  return (
    <>
      <FieldRow label={t("tts.text")} align="start">
        <InspectorTextarea value={text} onChange={(e) => onTextChange(e.currentTarget.value)} />
      </FieldRow>
      <FieldRow label={t("tts.speaker")}>
        <InspectorSelect
          data={options}
          value={speakerId !== null && speakerId !== 0 ? String(speakerId) : null}
          onChange={(value) => value && onSpeakerIdChange(Number(value))}
          disabled={loading || failed || options.length === 0}
          placeholder={placeholder}
        />
      </FieldRow>
      {failed && (
        <Text size="xs" c="red">
          {t("tts.speakersLoadFailed")}
        </Text>
      )}
    </>
  );
}
