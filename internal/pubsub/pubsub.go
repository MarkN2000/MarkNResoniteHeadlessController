// Package pubsub は型 T の値を購読者へファンアウトする汎用ブロードキャスタを提供する。
// SSE 配信のように「1つの出来事を、接続中の全購読者へ同時に届ける」用途に使う。
// 送信は非ブロッキング（遅い購読者向けにはドロップ）＝1つの遅い購読者が全体を詰まらせない。
//
// 元は internal/headless 内の private な hub[T] だったが、headless（ログ/状態）に加え
// steam（DL進捗/ログ）でも同じ仕組みが要るため共有パッケージへ抽出した。
package pubsub

import "sync"

// Hub は型 T の値を購読者へファンアウトする汎用ブロードキャスタ。
type Hub[T any] struct {
	mu   sync.Mutex
	subs map[chan T]struct{}
}

// NewHub は空の Hub を生成する。
func NewHub[T any]() *Hub[T] {
	return &Hub[T]{subs: make(map[chan T]struct{})}
}

// Publish は全購読者へ v を配る。バッファが満杯の購読者にはドロップする（非ブロッキング）。
func (h *Hub[T]) Publish(v T) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- v:
		default:
		}
	}
}

// Subscribe は新しい購読チャンネル（バッファ buf）を登録して返す。
func (h *Hub[T]) Subscribe(buf int) chan T {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan T, buf)
	h.subs[ch] = struct{}{}
	return ch
}

// Unsubscribe は購読チャンネルを解除して閉じる。
func (h *Hub[T]) Unsubscribe(ch chan T) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}
