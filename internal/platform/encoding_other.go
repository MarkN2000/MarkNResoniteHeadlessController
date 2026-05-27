//go:build !windows

package platform

import "golang.org/x/text/encoding"

// osConsoleEncoding はWindows以外（Linux/macOS）ではUTF-8パススルー（nil）を返す。
// 実機確定: Linux=UTF-8（docs/resonite-domain-facts.md §7）。
func osConsoleEncoding() encoding.Encoding { return nil }
