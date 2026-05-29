package headless

import (
	"reflect"
	"testing"
)

// テストデータは scripts/empirical-capture/fixtures/2026-05-28-windows-multiworld.log の
// 該当ブロックを抽出したもの（プロンプト接頭辞は除去済の前提）。

func TestParseWorlds(t *testing.T) {
	// 実機採取（2026-05-28 Windows Beta 2026.5.27.1300, 2ワールド・Private）
	input := []string{
		"[0] MRHC Test World A               Users: 1\tPresent: 0\tAccessLevel: Private\tMaxUsers: 4",
		"[1] MRHC Test World B               Users: 1\tPresent: 0\tAccessLevel: Private\tMaxUsers: 4",
	}
	got := ParseWorlds(input)
	want := []World{
		{Index: 0, Name: "MRHC Test World A", Users: 1, Present: 0, AccessLevel: "Private", MaxUsers: 4},
		{Index: 1, Name: "MRHC Test World B", Users: 1, Present: 0, AccessLevel: "Private", MaxUsers: 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("worlds parse mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

// ambient ログが混在しても応答行が取れること（プロンプト剥がしは Driver 済の前提で、
// パーサは ambient を regex 非一致で無視する責務のみ）。
func TestParseWorldsHandlesAmbient(t *testing.T) {
	input := []string{
		"BOOTSTRAP: Running userspace bootstrap",
		"User Joined Userspace. Username: MARKNPC_MAIN, UserID: ...",
		"Loading from URI: local://...",
		"Updated: 0001/01/01 0:00:00 -> 2026/05/28 8:19:20",
		"World running...",
		"[0] MRHC Test World A               Users: 1\tPresent: 0\tAccessLevel: Private\tMaxUsers: 4",
		"[1] MRHC Test World B               Users: 1\tPresent: 0\tAccessLevel: Private\tMaxUsers: 4",
	}
	got := ParseWorlds(input)
	if len(got) != 2 {
		t.Fatalf("ambient 混在で 2 worlds 取れるべき: got %d - %+v", len(got), got)
	}
	if got[0].Name != "MRHC Test World A" || got[0].Index != 0 {
		t.Fatalf("World 0 mismatch: %+v", got[0])
	}
	if got[1].Name != "MRHC Test World B" || got[1].Index != 1 {
		t.Fatalf("World 1 mismatch: %+v", got[1])
	}
}

func TestParseStatusHandlesAmbient(t *testing.T) {
	input := []string{
		"Some ambient log line during status response",
		"Name: MRHC Test World A",
		"SessionID: S-test-1234",
		"Current Users: 1",
	}
	got := ParseStatus(input)
	if got.Name != "MRHC Test World A" {
		t.Fatalf("Name 行が parse されるべき: %+v", got)
	}
	if got.SessionID != "S-test-1234" || got.CurrentUsers != 1 {
		t.Fatalf("通常行も parse されるべき: %+v", got)
	}
}

// リッチテキスト（'>' を含む）/ ':' を含む名前でも status が正しく parse されること。
// 旧来の貪欲プロンプト剥がしのバグ回帰防止（Driver が実プロンプトを剥がした後の綺麗な行を想定）。
func TestParseStatusRichTextValues(t *testing.T) {
	input := []string{
		"Name: 日本語 <color=red>赤</color> <s>取消</s>",
		"Description: 1行目<br>2行目 <color=#3399ff>青</color>",
		"Access Level: Private",
	}
	got := ParseStatus(input)
	if got.Name != "日本語 <color=red>赤</color> <s>取消</s>" {
		t.Fatalf("rich text を含む Name が壊れた: %q", got.Name)
	}
	if got.Description != "1行目<br>2行目 <color=#3399ff>青</color>" {
		t.Fatalf("rich text を含む Description が壊れた: %q", got.Description)
	}
}

func TestParseWorldsIgnoresNoise(t *testing.T) {
	// パーサは正規表現に当たらない行を自然に無視する
	input := []string{
		"this is ambient",
		"[0] World A               Users: 2\tPresent: 1\tAccessLevel: LAN\tMaxUsers: 16",
		"random log line",
	}
	got := ParseWorlds(input)
	if len(got) != 1 {
		t.Fatalf("ambient行は無視されるべき: got %d entries", len(got))
	}
	if got[0].Name != "World A" || got[0].Users != 2 || got[0].MaxUsers != 16 {
		t.Fatalf("parse mismatch: %+v", got[0])
	}
}

func TestParseStatus(t *testing.T) {
	// 実機採取（2026-05-28 Windows） focus 0 → status
	input := []string{
		"Name: MRHC Test World A",
		"SessionID: S-019e6cb1-6670-7f47-aaea-4f4fe830df12",
		"Current Users: 1",
		"Present Users: 0",
		"Max Users: 4",
		"Uptime: 00:00:21.4205512",
		"Access Level: Private",
		"Hidden from listing: True",
		"Mobile Friendly: False",
		"Description: empirical capture",
		"Tags: ",
		"Users: MARKNPC_MAIN",
		"ResoniteLink: off",
	}
	got := ParseStatus(input)
	want := WorldStatus{
		Name:              "MRHC Test World A",
		SessionID:         "S-019e6cb1-6670-7f47-aaea-4f4fe830df12",
		CurrentUsers:      1,
		PresentUsers:      0,
		MaxUsers:          4,
		Uptime:            "00:00:21.4205512",
		AccessLevel:       "Private",
		HiddenFromListing: true,
		MobileFriendly:    false,
		Description:       "empirical capture",
		Tags:              nil,
		Users:             []string{"MARKNPC_MAIN"},
		ResoniteLink:      "off",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status parse mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

func TestParseStatusUnknownKeyTolerated(t *testing.T) {
	// 未知Keyが混じってもパース全体は落ちず、既知部分は埋まる
	input := []string{
		"Name: hello",
		"FutureKey: some-value",
		"Current Users: 3",
	}
	got := ParseStatus(input)
	if got.Name != "hello" || got.CurrentUsers != 3 {
		t.Fatalf("既知Keyのパースは継続すべき: %+v", got)
	}
}

func TestParseStatusMultiUsersAndTags(t *testing.T) {
	input := []string{
		"Tags: alpha, beta, gamma",
		"Users: alice, bob , carol",
	}
	got := ParseStatus(input)
	wantTags := []string{"alpha", "beta", "gamma"}
	wantUsers := []string{"alice", "bob", "carol"}
	if !reflect.DeepEqual(got.Tags, wantTags) {
		t.Fatalf("tags: got=%v want=%v", got.Tags, wantTags)
	}
	if !reflect.DeepEqual(got.Users, wantUsers) {
		t.Fatalf("users: got=%v want=%v", got.Users, wantUsers)
	}
}

func TestParseUsers(t *testing.T) {
	// 実機採取（2026-05-28 Windows）ID は空文字（ヘッドレス自身ユーザー）
	input := []string{
		"MARKNPC_MAIN\tID: \tRole: Admin\tPresent: False\tPing: 0 ms\tFPS: 59.65997\tSilenced: False",
	}
	got := ParseUsers(input)
	want := []UserInfo{
		{Name: "MARKNPC_MAIN", ID: "", Role: "Admin", Present: false, PingMs: 0, FPS: 59.65997, Silenced: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("users parse mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

func TestParseUsersWithID(t *testing.T) {
	// 実アカウントユーザー（ID あり）の想定
	input := []string{
		"alice\tID: U-alice-1234\tRole: Builder\tPresent: True\tPing: 42 ms\tFPS: 90.0\tSilenced: False",
		"bob with space\tID: U-bob-xyz\tRole: Guest\tPresent: True\tPing: 100 ms\tFPS: 60.5\tSilenced: True",
	}
	got := ParseUsers(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 users, got %d", len(got))
	}
	if got[0].Name != "alice" || got[0].ID != "U-alice-1234" || got[0].PingMs != 42 || got[0].FPS != 90.0 || got[0].Present != true {
		t.Fatalf("alice mismatch: %+v", got[0])
	}
	// 名前に空白を含むケース（TAB区切りで対応可能）
	if got[1].Name != "bob with space" || !got[1].Silenced {
		t.Fatalf("bob with space mismatch: %+v", got[1])
	}
}

func TestParseListBansEmpty(t *testing.T) {
	got := ParseListBans([]string{})
	if len(got) != 0 {
		t.Fatalf("空入力は空 slice: %v", got)
	}
}

func TestParseListBans(t *testing.T) {
	// 旧コード書式（実機未確認だが旧 regex 互換性を回帰テスト）
	input := []string{
		"[0]     Username: alice UserID: U-alice-1234   MachineIds: m1 m2",
	}
	got := ParseListBans(input)
	want := []BanEntry{
		{Index: 0, Username: "alice", UserID: "U-alice-1234", MachineIDs: []string{"m1", "m2"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listbans parse mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

// ParseFriendRequests のテスト（v1 互換実装）
func TestParseFriendRequests_Basic(t *testing.T) {
	// プレーンなユーザー名行 (実書式の代表例)
	got := ParseFriendRequests([]string{"alice", " bob ", "", "carol"})
	want := []string{"alice", "bob", "carol"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestParseFriendRequests_EmptyResponse(t *testing.T) {
	// 空のレスポンス (pending 無し)
	got := ParseFriendRequests([]string{})
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestParseFriendRequests_PromptOnlyLineExcluded(t *testing.T) {
	// プロンプトのみ ('>' で終わる) 行は除外 (v1 と同じ)
	got := ParseFriendRequests([]string{"alice", "MyHeadless>", "bob"})
	want := []string{"alice", "bob"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

// 2026-05-28 実機採取で確証 (MARKNPC_SUB2 アカウント、手動 stdin で friendrequests 実行)。
// 実際の format: 1 username/行のプレーンテキスト。usernames はハイフン・アンダースコア
// を含み得る。先頭行の prompt-glue は Driver(stripExactPrompt) が剥がす前提で、ここでは
// 綺麗な行を渡す。ユーザー名は匿名化済（実際は別の Resonite アカウント名 4 件）。
func TestParseFriendRequests_RealFormatEmpirical(t *testing.T) {
	input := []string{
		"alice",      // 先頭行（Driver が prompt 剥がし済の想定）
		"bob_user",   // underscore
		"carol-dash", // ハイフン含む
		"dave2024",   // 数字含む
	}
	got := ParseFriendRequests(input)
	want := []string{"alice", "bob_user", "carol-dash", "dave2024"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("実機 format 互換性失敗: got=%v want=%v", got, want)
	}
}

func TestParseFriendRequests_AmbientNotOverStripped(t *testing.T) {
	// ParseFriendRequests はプロンプト剥がしをせず（Driver の stripExactPrompt 済が前提）、
	// "Updated: A -> B" のような ambient 行も '>' を含むが誤剥がしされずそのまま残る。
	// → 明らかにユーザー名でない形で残るので UI 側で見分け可能 (v1 と同じ挙動)。
	got := ParseFriendRequests([]string{
		"Updated: 0001/01/01 0:00:00 -> 2026/05/28 8:19:20",
		"alice",
		"BOOTSTRAP: Running userspace bootstrap",
	})
	// ambient は剥がされず明らかな非ユーザー名形で残る (v1 と同じ受容)
	if len(got) != 3 {
		t.Fatalf("v1 互換: ambient も含めて 3 件、got=%v", got)
	}
	if got[1] != "alice" {
		t.Fatalf("alice が壊れている: %v", got)
	}
	// 重要: ambient の '>' を含む行が "B" に化けていないこと
	for _, s := range got {
		if s == "B" {
			t.Fatalf("ambient の '>' を含む行が誤剥がしされて 'B' に化けた: %v", got)
		}
	}
}

// 注: 旧 stripLineLeadingPrompts/safeStripLeadingPrompts は廃止。
// プロンプト剥がしは Driver 側 stripExactPrompt が担い、その単体テストは
// executor_test.go の TestStripExactPrompt にある（'>' を含む値・':' を含む名前も両立）。
