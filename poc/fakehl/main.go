// Command fakehl is a fake Resonite headless used for:
//   - smoke-testing the PoC controller on machines without a real headless
//   - integration testing the structured Console Driver (internal/headless)
//
// It mimics the relevant behaviors observed in the empirical capture
// (2026-05-28 Windows): multiple worlds, focus state, per-command response
// formats, prompt without trailing newline, silent commands, Unknown command.
//
// Default flags are tuned for integration tests (ambient OFF for deterministic
// output). For PoC smoke testing with periodic ambient ticks, pass `-ambient=true`.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// world はテスト用の最小状態。実 Resonite の SessionInfo より粗いが、
// MRHC 構造化 Driver/Parser の検証には十分。
type world struct {
	Name        string
	Users       int
	Present     int
	AccessLevel string
	MaxUsers    int
}

// state はプロセス全体の可変状態。focus は worlds の index。
type state struct {
	worlds   []world
	focused  int
	bans     []banEntry
	requests []string
	users    []userRow
}

type banEntry struct {
	Username   string
	UserID     string
	MachineIDs []string
}

type userRow struct {
	Name     string
	ID       string
	Role     string
	Present  bool
	PingMs   int
	FPS      float64
	Silenced bool
}

func newState(n int) *state {
	s := &state{focused: 0}
	for i := 0; i < n; i++ {
		s.worlds = append(s.worlds, world{
			Name:        fmt.Sprintf("Fake World %d", i),
			Users:       1,
			Present:     0,
			AccessLevel: "Private",
			MaxUsers:    4,
		})
	}
	s.users = []userRow{
		{Name: "FakeUser", ID: "", Role: "Admin", PingMs: 0, FPS: 60.0},
	}
	return s
}

func main() {
	ambient := flag.Bool("ambient", false, "emit ambient log lines (default OFF for tests; pass -ambient=true for PoC smoke)")
	ambientInterval := flag.Duration("ambient-interval", 3*time.Second, "ambient log interval")
	worldsCount := flag.Int("worlds", 2, "initial worlds count")
	banner := flag.Bool("banner", true, "print startup banner / 'World running' lines")
	flag.Parse()

	s := newState(*worldsCount)

	if *banner {
		fmt.Println("Fake Headless starting...")
		for i := range s.worlds {
			fmt.Printf("World running... (%d: %s)\n", i, s.worlds[i].Name)
		}
	}

	if *ambient {
		go ambientLoop(*ambientInterval)
	}

	// プロンプトは改行なしで出す（実機と同じ挙動）
	writePrompt(s)

	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		handleCommand(s, sc.Text())
		writePrompt(s)
	}
}

func writePrompt(s *state) {
	fmt.Printf("%s>", s.worlds[s.focused].Name)
}

func ambientLoop(interval time.Duration) {
	i := 0
	for {
		time.Sleep(interval)
		i++
		fmt.Printf("[ambient] tick %d\n", i)
	}
}

// handleCommand 1コマンドを処理。直後にプロンプトを呼び出し側で出す前提（このfnは出さない）。
func handleCommand(s *state, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	parts := strings.SplitN(line, " ", 2)
	cmd := parts[0]
	rest := ""
	if len(parts) > 1 {
		rest = parts[1]
	}

	switch cmd {
	case "worlds":
		for i, w := range s.worlds {
			// name 30字幅にスペースパディングしてTAB区切り（実機と同形式）
			padded := w.Name
			for len(padded) < 30 {
				padded += " "
			}
			fmt.Printf("[%d] %s\tUsers: %d\tPresent: %d\tAccessLevel: %s\tMaxUsers: %d\n",
				i, padded, w.Users, w.Present, w.AccessLevel, w.MaxUsers)
		}
	case "focus":
		idx, err := strconv.Atoi(strings.TrimSpace(rest))
		if err != nil || idx < 0 || idx >= len(s.worlds) {
			fmt.Println("Unknown command")
			return
		}
		s.focused = idx
		// silent（プロンプトだけ変化）
	case "status":
		w := s.worlds[s.focused]
		fmt.Printf("Name: %s\n", w.Name)
		fmt.Printf("SessionID: S-fake-%d-0000-0000-0000-000000000000\n", s.focused)
		fmt.Printf("Current Users: %d\n", w.Users)
		fmt.Printf("Present Users: %d\n", w.Present)
		fmt.Printf("Max Users: %d\n", w.MaxUsers)
		fmt.Printf("Uptime: 00:01:23.4567890\n")
		fmt.Printf("Access Level: %s\n", w.AccessLevel)
		fmt.Printf("Hidden from listing: True\n")
		fmt.Printf("Mobile Friendly: False\n")
		fmt.Printf("Description: fake world for testing\n")
		fmt.Printf("Tags: \n")
		// users 列名一覧（カンマ区切り）
		names := make([]string, 0, len(s.users))
		for _, u := range s.users {
			names = append(names, u.Name)
		}
		fmt.Printf("Users: %s\n", strings.Join(names, ", "))
		fmt.Printf("ResoniteLink: off\n")
	case "users":
		for _, u := range s.users {
			fmt.Printf("%s\tID: %s\tRole: %s\tPresent: %v\tPing: %d ms\tFPS: %g\tSilenced: %v\n",
				u.Name, u.ID, u.Role, boolPy(u.Present), u.PingMs, u.FPS, boolPy(u.Silenced))
		}
	case "name":
		// rest は "新名" の形（引用符付きが多い）
		newName := strings.Trim(rest, `" `)
		if newName != "" {
			s.worlds[s.focused].Name = newName
		}
		// silent
	case "maxusers":
		n, err := strconv.Atoi(strings.TrimSpace(rest))
		if err == nil {
			s.worlds[s.focused].MaxUsers = n
		}
		// silent
	case "accesslevel":
		level := strings.TrimSpace(rest)
		s.worlds[s.focused].AccessLevel = level
		fmt.Printf("World %s now has access level %s\n", s.worlds[s.focused].Name, level)
	case "listbans":
		for i, b := range s.bans {
			fmt.Printf("[%d]     Username: %s UserID: %s   MachineIds: %s\n",
				i, b.Username, b.UserID, strings.Join(b.MachineIDs, " "))
		}
		// 空なら silent
	case "friendrequests":
		for _, r := range s.requests {
			fmt.Println(r)
		}
		// 空なら silent
	case "hang":
		// テスト用: 永久ブロック（プロンプトを返さない）→ Exec timeout / ProcessGone 検証
		time.Sleep(time.Hour)
	case "shutdown":
		fmt.Println("Exiting. Save Homes: False")
		fmt.Println("Saving all settings")
		os.Exit(0)
	default:
		fmt.Println("Unknown command")
	}
}

// boolPy returns "True"/"False" — Resonite のbool表記に合わせる（実機観測）。
func boolPy(b bool) string {
	if b {
		return "True"
	}
	return "False"
}
