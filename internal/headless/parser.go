package headless

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// 各構造化コマンドの応答パーサ。入力 lines は Executor 側で「行頭の実プロンプト」
// （stripExactPrompt）も剥がし済みの綺麗な行。各パーサは Key:Value / 行 regex に
// 当てるだけの純粋関数。ambient/無関係行は regex に当たらず自然に無視される。
//
// === 既知の理論的限界（実害低）===
//   - splitCommaList で `status.Users`/`Tags` を分割: ユーザー名/タグに ',' を
//     含むケースは誤分割。Resonite のユーザー名仕様は寛容なため理論上ありうるが、
//     2026-05-28 実機採取の範囲では発生せず。検証可能なケースが出たら対応。
//   - プロンプト末尾検出 (`waitComplete`): ワールド名末尾が '>' のとき誤検出。
//     極めて稀。Resonite UI で `>` で終わる名前を作るのは通常想定外。
//   - `worldsLineRe` の `(.+?)` 名前: name に "Users:" 含むと誤分割。
//     名前に「 Users: 」というスペース＋コロン列を入れるのは想定外で実用上問題なし。

// userIDPat は Resonite UserID（"U-" + 英数/_/-）のトークンパターン。
// listbans / login など UserID を抽出する箇所で共有し charset を一致させる（SanitizeToken とも整合）。
const userIDPat = `U-[A-Za-z0-9_-]+`

var (
	// worlds: `[<idx>] <name padded>\tUsers: N\tPresent: N\tAccessLevel: L\tMaxUsers: N`
	// 名前と Users の間は空白パディング、それ以降は TAB 区切りが実機観測。
	// \s+ は両方を吸収する。
	worldsLineRe = regexp.MustCompile(`^\[(\d+)\]\s+(.+?)\s+Users:\s+(\d+)\s+Present:\s+(\d+)\s+AccessLevel:\s+(\S+)\s+MaxUsers:\s+(\d+)`)

	// status: `Key: Value` の行ベース。Key は ':' 区切り左側を trim。
	statusKVRe = regexp.MustCompile(`^([^:]+):\s*(.*)$`)

	// users: `Name\tID: <id|empty>\tRole: R\tPresent: B\tPing: N ms\tFPS: F\tSilenced: B`
	// TAB 区切り（名前に空白が含まれても破綻しない）。ID は空文字を許容。
	usersLineRe = regexp.MustCompile(`^([^\t]+)\tID:\s*([^\t]*)\tRole:\s+([^\t]+)\tPresent:\s+(True|False)\tPing:\s+(\d+)\s+ms\tFPS:\s+([0-9.]+)\tSilenced:\s+(True|False)$`)

	// listbans: `[<idx>] Username: U UserID: U-... MachineIds: ...`
	listbansLineRe = regexp.MustCompile(`^\[(\d+)\]\s+Username:\s+(\S+)\s+UserID:\s+(` + userIDPat + `)\s+MachineIds:\s+(.*)$`)
)

// statusUnknownKeysWarned: status の未知 Key を「初回1回だけ」警告するため。
// 出力書式がバージョンで増減してもパースは落とさず、運用者に1度だけ気付かせる。
var statusUnknownKeysWarned sync.Map

// ParseWorlds は worlds コマンドの応答行から World 一覧を構築する。
func ParseWorlds(lines []string) []World {
	out := make([]World, 0, len(lines))
	for _, line := range lines {
		m := worldsLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		idx, _ := strconv.Atoi(m[1])
		users, _ := strconv.Atoi(m[3])
		present, _ := strconv.Atoi(m[4])
		max, _ := strconv.Atoi(m[6])
		out = append(out, World{
			Index:       idx,
			Name:        strings.TrimSpace(m[2]),
			Users:       users,
			Present:     present,
			AccessLevel: m[5],
			MaxUsers:    max,
		})
	}
	return out
}

// ParseStatus は status コマンドの応答行から WorldStatus を構築する。
//
// === ambient ログ混入への耐性（2026-06-08 実機観測で強化）===
// Resonite は status コマンド出力の直前に非同期ログを stdout へ流すことがあり、
// その行が応答窓に紛れ込む（[structured-driver] の「未知Key」警告の発生源）。
// 実機観測された混入パターン（fixtures/2026-06-08-status-ambient.log）:
//   - "SIGNALR: BroadcastStatus - ..." + TAB インデントの子項目群（UserStatus ダンプ）
//   - "Running refresh on: ..."
//   - "SIGNALR: BroadcastSession SessionInfo. ... Name: X, ..."（1行・値中に Name: を含む）
// これらは全て本物の status ブロック（Name: 〜 ResoniteLink:）より前に出現する。
//
// 対策:
//   1. 最初の「既知 Status Key」が現れるまでの行を読み飛ばす（前方 ambient を除外）。
//      Name 固定ではなく既知 Key で起点を取るため、項目順が変わっても壊れない。
//   2. 行頭がインデント（TAB/空白）の行は ambient 子項目とみなし無視（保険）。
//   3. 既知 Key は first-value-wins（万一の重複・衝突でも先頭=本物を保持）。
//   4. ブロック開始後の未知 Key のみ初回1回だけ警告（Resonite の真の新項目検知は維持）。
func ParseStatus(lines []string) WorldStatus {
	var s WorldStatus
	seen := make(map[string]bool)
	started := false // 最初の既知 Key を観測したら true（status ブロック開始）
	for _, line := range lines {
		if isIndentedLine(line) {
			continue // ambient ダンプの TAB インデント子項目（"\tUserSessionId: ..." 等）
		}
		m := statusKVRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := strings.TrimSpace(m[1])
		val := strings.TrimSpace(m[2])
		if seen[key] {
			continue // first-value-wins
		}
		switch {
		case assignStatusField(&s, key, val):
			seen[key] = true
			started = true
		case started:
			// ブロック内に現れた未知 Key = Resonite の真の新項目候補
			seen[key] = true
			warnUnknownStatusKey(key)
		default:
			// ブロック開始前の未知 Key = ambient → 黙って読み飛ばす（警告しない）
		}
	}
	return s
}

// assignStatusField は既知 Status Key なら s に値を書いて true、未知 Key なら false を返す。
// 既知 Key 一覧の唯一の定義箇所（ParseStatus の起点判定もこの真偽値に従う）。
func assignStatusField(s *WorldStatus, key, val string) bool {
	switch key {
	case "Name":
		s.Name = val
	case "SessionID":
		s.SessionID = val
	case "Current Users":
		s.CurrentUsers, _ = strconv.Atoi(val)
	case "Present Users":
		s.PresentUsers, _ = strconv.Atoi(val)
	case "Max Users":
		s.MaxUsers, _ = strconv.Atoi(val)
	case "Uptime":
		s.Uptime = val
	case "Access Level":
		s.AccessLevel = val
	case "Hidden from listing":
		s.HiddenFromListing = parseHeadlessBool(val)
	case "Mobile Friendly":
		s.MobileFriendly = parseHeadlessBool(val)
	case "Description":
		s.Description = val
	case "Tags":
		s.Tags = splitCommaList(val)
	case "Users":
		s.Users = splitCommaList(val)
	case "ResoniteLink":
		s.ResoniteLink = val
	default:
		return false
	}
	return true
}

// isIndentedLine は行頭が空白/TAB かを返す。ambient ダンプの子項目（"\tUserSessionId: ..."）は
// インデントされ、本物の status 行は常に非インデントなので、これで両者を分離する。
func isIndentedLine(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

// ParseUsers は users コマンドの応答行から UserInfo 一覧を構築する。
func ParseUsers(lines []string) []UserInfo {
	out := make([]UserInfo, 0, len(lines))
	for _, line := range lines {
		m := usersLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ping, _ := strconv.Atoi(m[5])
		fps, _ := strconv.ParseFloat(m[6], 64)
		out = append(out, UserInfo{
			Name:     m[1],
			ID:       m[2],
			Role:     m[3],
			Present:  m[4] == "True",
			PingMs:   ping,
			FPS:      fps,
			Silenced: m[7] == "True",
		})
	}
	return out
}

// ParseListBans は listbans コマンドの応答行から BanEntry 一覧を構築する。
// 空リストの場合は出力行が無い（プロンプトのみ）→ 空 slice を返す。
func ParseListBans(lines []string) []BanEntry {
	out := make([]BanEntry, 0, len(lines))
	for _, line := range lines {
		m := listbansLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		idx, _ := strconv.Atoi(m[1])
		machineIDs := strings.Fields(strings.TrimSpace(m[4]))
		out = append(out, BanEntry{
			Index:      idx,
			Username:   m[2],
			UserID:     m[3],
			MachineIDs: machineIDs,
		})
	}
	return out
}

// ParseFriendRequests は friendrequests コマンドの応答行からユーザー名一覧を構築する。
//
// 実装方針 (v1 Node実装 parseFriendRequestsOutput と同じ単純戦略):
//   - 各 line を trim
//   - 空行 / プロンプトで終わる行 ('>') を除外
//   - 残りを「pending friend request の username」として返す
//
// 行頭のプロンプト接頭辞は Driver 側 (stripExactPrompt) が剥がし済みなので、
// ここでは trim + 空行/プロンプト('>'終端)除外だけを行う。
//
// === 既知の限界 ===
// v1 と同じ限界を継承:
//   - boot 直後など ambient ログが大量に流れる状況では、ambient 行が「friend request
//     のユーザー名」として混入する可能性がある (v1 でも同じ問題)。
//   - production の steady state (mrhc 24/7 稼働後) では ambient が希少なので
//     実用上問題にならない。boot 直後の呼び出しは結果を信頼しすぎないこと。
//   - 完全に信頼できる検証は、第三者からの pending friend request の実書式を
//     採取してから (現状: 未採取)。実書式が判明したらより厳密な regex に差し替え可能。
func ParseFriendRequests(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasSuffix(t, ">") {
			continue // プロンプトのみの行（念のための保険。通常は Driver が剥がし済）
		}
		out = append(out, t)
	}
	return out
}

// --- ヘルパ ---

func parseHeadlessBool(v string) bool { return v == "True" }

func splitCommaList(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func warnUnknownStatusKey(key string) {
	if _, loaded := statusUnknownKeysWarned.LoadOrStore(key, true); loaded {
		return // 既に警告済
	}
	log.Printf("[structured-driver] status の未知Key %q を観測 (無視。Resoniteのバージョン変更で増えた可能性)", key)
}
