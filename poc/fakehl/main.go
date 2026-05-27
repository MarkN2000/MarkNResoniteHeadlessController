// Command fakehl is a fake Resonite headless used to smoke-test the PoC
// controller on machines without a real headless (e.g. the Windows dev box).
// It mimics the relevant behaviours: a startup banner, periodic ambient log
// lines, a prompt, and a few command responses — including a UTF-8 Japanese
// line so we can verify encoding end to end.
package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

func main() {
	fmt.Println("Fake Headless starting...")
	fmt.Println("World running (UTF-8 確認: 日本語テスト ✓)")

	// Ambient log lines unrelated to any command (mimics Resonite's stream).
	go func() {
		i := 0
		for {
			time.Sleep(3 * time.Second)
			i++
			fmt.Printf("[ambient] tick %d\n", i)
		}
	}()

	sc := bufio.NewScanner(os.Stdin)
	fmt.Print("MyWorld>")
	for sc.Scan() {
		switch line := sc.Text(); line {
		case "worlds":
			fmt.Println("[0] テストワールド\tUsers: 2\tPresent: 1\tAccessLevel: LAN\tMaxUsers: 16")
		case "status":
			fmt.Println("Name: テストワールド")
			fmt.Println("SessionID: S-00000000-0000-0000-0000-000000000000")
			fmt.Println("Current Users: 2")
		case "shutdown":
			fmt.Println("Shutting down...")
			return
		default:
			fmt.Printf("echo: %s\n", line)
		}
		fmt.Print("MyWorld>")
	}
}
