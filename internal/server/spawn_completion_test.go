package server

import (
	"errors"
	"strings"
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
)

type fakeSpawnTx struct {
	lines []string
	err   error
	cmd   string
}

func (f *fakeSpawnTx) Exec(cmd string, _ ...headless.ExecOption) ([]string, error) {
	f.cmd = cmd
	return f.lines, f.err
}

func TestExecTemporarySpawn(t *testing.T) {
	const itemURL = "resrec:///U-MarkN/R-abc"

	t.Run("完了行を確認できれば成功", func(t *testing.T) {
		exec := &fakeSpawnTx{lines: []string{"Spawned item from URL: " + itemURL}}
		if err := execTemporarySpawn(exec, itemURL); err != nil {
			t.Fatalf("スポーン成功を判定できない: %v", err)
		}
		if exec.cmd != `spawn "`+itemURL+`" true false` {
			t.Fatalf("スポーンコマンドが想定外: %q", exec.cmd)
		}
	})

	t.Run("完了行が無ければ失敗", func(t *testing.T) {
		exec := &fakeSpawnTx{lines: []string{"Spawning item from URL: " + itemURL}}
		err := execTemporarySpawn(exec, itemURL)
		if err == nil || !strings.Contains(err.Error(), "スポーン完了を確認できませんでした") {
			t.Fatalf("完了未確認を失敗にできていない: %v", err)
		}
	})

	t.Run("実行エラーを保持", func(t *testing.T) {
		want := errors.New("spawn timeout")
		exec := &fakeSpawnTx{err: want}
		if err := execTemporarySpawn(exec, itemURL); !errors.Is(err, want) {
			t.Fatalf("実行エラーが保持されていない: %v", err)
		}
	})
}
