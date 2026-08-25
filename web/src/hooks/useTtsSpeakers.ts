import { useEffect, useState } from "react";
import * as api from "../api";
import type { TtsSpeaker } from "../api";

export interface TtsSpeakersState {
  voices: TtsSpeaker[];
  loading: boolean;
  failed: boolean;
}

// TTS 入力が必要な間だけ話者一覧を取得する。失敗と空一覧は別の状態として保持する。
export function useTtsSpeakers(enabled: boolean): TtsSpeakersState {
  const [state, setState] = useState<TtsSpeakersState>({ voices: [], loading: false, failed: false });

  useEffect(() => {
    if (!enabled) {
      setState({ voices: [], loading: false, failed: false });
      return;
    }
    let alive = true;
    setState({ voices: [], loading: true, failed: false });
    void api.getTtsSpeakers().then(({ voices, ok }) => {
      if (alive) setState({ voices, loading: false, failed: !ok });
    });
    return () => {
      alive = false;
    };
  }, [enabled]);

  return state;
}
