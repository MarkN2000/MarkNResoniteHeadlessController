package headless

import (
	"context"
	"fmt"
	"time"
)

// WorldsService は worlds 巡回ロジックを共通化する薄いサービス層。
// 設計: docs/design/structured-driver.md §7
//
// 用途:
//   - List(): userZero 判定（合計人数）、ダッシュボードのワールド一覧表示
//   - ForEach(): 各ワールドのユーザー収集、事前アクション、セッション変更
//     ForEach は原子的グループ（ExecGroup）で実行されるため、巡回中に他の
//     構造化コマンドが focus を割り込むことはない。

type WorldsService interface {
	// List は worlds コマンドを1回叩いて全ワールドの要約を返す。
	// userZero 判定（合計人数=0）はこの結果から計算可能で、focus 巡回は不要。
	List(ctx context.Context) ([]World, error)

	// ForEach は各ワールドを focus してから fn を呼ぶ。
	// 巡回は原子的グループ内で行われ、他の構造化コマンドが間に割り込まない。
	// fn が non-nil error を返すと巡回を中断してその error を返す。
	ForEach(ctx context.Context, fn func(w World, s Scope) error) error
}

// Scope は ForEach の fn 内で使う「既に focus 済」の Exec ハンドル。
// fn は scope.Exec("status") のように、focus 切替を気にせずコマンドを発行できる。
type Scope interface {
	Exec(cmd string, opts ...ExecOption) ([]string, error)
	World() World
}

type scopeImpl struct {
	tx Tx
	w  World
}

func (s *scopeImpl) Exec(cmd string, opts ...ExecOption) ([]string, error) {
	return s.tx.Exec(cmd, opts...)
}
func (s *scopeImpl) World() World { return s.w }

type worldsService struct {
	d *Driver
}

// NewWorldsService は Driver を使う WorldsService を返す。
func NewWorldsService(d *Driver) WorldsService {
	return &worldsService{d: d}
}

func (s *worldsService) List(ctx context.Context) ([]World, error) {
	lines, err := s.d.Exec(ctx, "worlds")
	if err != nil {
		return ParseWorlds(lines), err // 部分結果も返す
	}
	return ParseWorlds(lines), nil
}

func (s *worldsService) ForEach(ctx context.Context, fn func(w World, s Scope) error) error {
	return s.d.ExecGroup(ctx, func(tx Tx) error {
		// 巡回直前に最新の worlds を取得（focus index ズレを最小化）。
		// 採取中に他のソースで focus が変わっても、ここから先は execMu を握ったまま。
		lines, err := tx.Exec("worlds")
		if err != nil {
			return fmt.Errorf("worlds: %w", err)
		}
		worlds := ParseWorlds(lines)
		for _, w := range worlds {
			if _, err := tx.Exec(fmt.Sprintf("focus %d", w.Index), WithTimeout(2*time.Second)); err != nil {
				return fmt.Errorf("focus %d: %w", w.Index, err)
			}
			if err := fn(w, &scopeImpl{tx: tx, w: w}); err != nil {
				return err
			}
		}
		return nil
	})
}
