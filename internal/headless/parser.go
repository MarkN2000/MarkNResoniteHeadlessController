package headless

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// 各構造化コマンドの応答パーサ。入力 lines は Executor 側で
//   1. 各行 \r\n trim 済（既存 decodeLine の仕様）
//   2. 先頭行のプロンプト接頭辞除去済（案X：「行頭から最初の '>' まで」を除去）
// である前提。ambient/無関係行は regex に当たらず自然に無視される。

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
	listbansLineRe = regexp.MustCompile(`^\[(\d+)\]\s+Username:\s+(\S+)\s+UserID:\s+(U-[A-Za-z0-9_-]+)\s+MachineIds:\s+(.*)$`)

	// leadingPromptsRe: 行頭の「連続プロンプト」をまとめて剥がす（案X拡張）。
	// 実機観測: 構造化コマンド前に「前回コマンドの後で出たプロンプト」が
	// 改行なしで lineBuf に残るため、新コマンドの最初の行は
	//   <prompt1>><prompt2>><content>
	// のように 1 個以上のプロンプトが連結することがある（特に ExecGroup で
	// silent コマンドを連続させた場合）。連続する「[^>]*>」をまとめて剥がす。
	// 注意: 構造化コマンドの応答1行目に '>' が含まれるとき過剰に剥がす可能性が
	// あるが、現在対応するコマンドの1行目は '>' を含まない（worlds=[…]、
	// status=Name:、users=ユーザー名 など）ため実害なし。
	leadingPromptsRe = regexp.MustCompile(`^([^>]*>)+`)
)

// statusUnknownKeysWarned: status の未知 Key を「初回1回だけ」警告するため。
// 出力書式がバージョンで増減してもパースは落とさず、運用者に1度だけ気付かせる。
var statusUnknownKeysWarned sync.Map

// ParseWorlds は worlds コマンドの応答行から World 一覧を構築する。
func ParseWorlds(lines []string) []World {
	out := make([]World, 0, len(lines))
	for _, line := range lines {
		cleaned := stripLineLeadingPrompts(line)
		m := worldsLineRe.FindStringSubmatch(cleaned)
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
// 知っている Key だけ構造体に写し、未知 Key は初回1回だけ警告して無視する
// （将来のバージョン変化への耐性）。
func ParseStatus(lines []string) WorldStatus {
	var s WorldStatus
	for _, line := range lines {
		cleaned := stripLineLeadingPrompts(line)
		m := statusKVRe.FindStringSubmatch(cleaned)
		if m == nil {
			continue
		}
		key := strings.TrimSpace(m[1])
		val := strings.TrimSpace(m[2])
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
			warnUnknownStatusKey(key)
		}
	}
	return s
}

// ParseUsers は users コマンドの応答行から UserInfo 一覧を構築する。
func ParseUsers(lines []string) []UserInfo {
	out := make([]UserInfo, 0, len(lines))
	for _, line := range lines {
		cleaned := stripLineLeadingPrompts(line)
		m := usersLineRe.FindStringSubmatch(cleaned)
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
		cleaned := stripLineLeadingPrompts(line)
		m := listbansLineRe.FindStringSubmatch(cleaned)
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
// 出力はユーザー名1人/行（プロンプト/空行は除外）。空リストは []string{} を返す。
func ParseFriendRequests(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
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

// stripLineLeadingPrompts は単一行の先頭にある「連続プロンプト」を除去する。
// 各パーサが per-line で呼ぶ。
//
// 必要な理由: Driver の collector は Exec 中に流れる全ての stdout 行を捕える
// （ambient/起動ログ含む）。そのため応答行は collector の任意の位置に現れ得る。
// stripPromptPrefix（lines[0] のみ）では不足で、応答らしい行が ambient と
// glue したパターン（例: "World A>[0] foo  Users: 1 ..."）を全行で剥がす必要がある。
//
// 注意: ambient 行に '>' が含まれると過剰に剥がすが、その場合は parser regex に
// 一致しないため最終結果に影響しない（無害な over-strip）。
func stripLineLeadingPrompts(line string) string {
	loc := leadingPromptsRe.FindStringIndex(line)
	if loc == nil {
		return line
	}
	return line[loc[1]:]
}

// stripPromptPrefix は応答の先頭行に限り、行頭の「連続プロンプト」を全て除去する。
// 案X（拡張）：プロンプト `<world>>` を generic に剥がし、パーサが `^` アンカー
// regex を使える状態にする。
//
// 単一プロンプト例:    "Fake World 0>[0] World A …" → "[0] World A …"
// 連続プロンプト例:    "Fake World 0>Fake World 1>Name: …" → "Name: …"
//   ※ ExecGroup で silent 系コマンドの直後に次コマンドが続くと、
//     前コマンドの prompt が lineBuf に残ったまま次の出力が連結するため。
//
// lines を mutate せず新しい slice を返す。
func stripPromptPrefix(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	first := lines[0]
	loc := leadingPromptsRe.FindStringIndex(first)
	if loc == nil {
		return lines
	}
	out := make([]string, len(lines))
	copy(out, lines)
	out[0] = first[loc[1]:]
	return out
}
