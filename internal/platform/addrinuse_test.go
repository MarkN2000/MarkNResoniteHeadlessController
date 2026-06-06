package platform

import (
	"errors"
	"net"
	"testing"
)

// TestIsAddrInUse は実際に二重 Listen して OS 実エラーで判定を確認する
// （Windows=WSAEADDRINUSE / Unix=EADDRINUSE の差異を吸収できているか）。
func TestIsAddrInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	_, err = net.Listen("tcp", ln.Addr().String())
	if err == nil {
		t.Fatal("同一アドレスの二重 Listen は失敗するはず")
	}
	if !IsAddrInUse(err) {
		t.Errorf("IsAddrInUse(実エラー) = false: %v", err)
	}

	if IsAddrInUse(errors.New("unrelated")) {
		t.Error("無関係のエラーで true になった")
	}
	if IsAddrInUse(nil) {
		t.Error("nil で true になった")
	}
}
