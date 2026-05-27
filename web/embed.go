// Package web はビルド済みフロントエンド静的資産をバイナリへ埋め込む。
// v1.0では暫定の最小UI(dist/index.html)。後続でReact+Viteのビルド成果物が
// dist/ を置き換える（埋め込みパスは不変）。フロント本体(src等)も web/ に置く。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS は dist/ をルートに見せる fs.FS を返す。
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist は必ず存在する（埋め込み済み）
	}
	return sub
}
