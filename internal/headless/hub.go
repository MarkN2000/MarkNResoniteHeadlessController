package headless

import "sync"

// hub は型Tの値を購読者へファンアウトする汎用ブロードキャスタ。
// 送信は非ブロッキング（遅い購読者向けにはドロップ）。
type hub[T any] struct {
	mu   sync.Mutex
	subs map[chan T]struct{}
}

func newHub[T any]() *hub[T] {
	return &hub[T]{subs: make(map[chan T]struct{})}
}

func (h *hub[T]) publish(v T) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- v:
		default:
		}
	}
}

func (h *hub[T]) subscribe(buf int) chan T {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan T, buf)
	h.subs[ch] = struct{}{}
	return ch
}

func (h *hub[T]) unsubscribe(ch chan T) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}
