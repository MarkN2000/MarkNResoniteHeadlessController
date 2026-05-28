// Command poc is a throwaway proof-of-concept that validates the core
// architecture of the MRHC v2 rewrite on both Windows and Linux:
//   - launching a child process (the Resonite headless) and piping its I/O
//   - reading stdout/stderr line by line (UTF-8)
//   - sending commands to the child's stdin
//   - serving an embedded SPA via embed.FS
//   - streaming the child's output to the browser over SSE
//
// Usage (Linux, real headless):
//
//	poc -cmd dotnet -- /path/to/Resonite/Headless/Resonite.dll -HeadlessConfig cfg.json
//
// Usage (local smoke test with the fake headless):
//
//	poc -cmd ./bin/fakehl.exe
package main

import (
	"bufio"
	"embed"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

//go:embed web/index.html
var webFS embed.FS

// logSink: empirical-capture helper. If -logfile is set, every published line is
// written verbatim here. This bypasses SSE's per-client drop behavior so capture
// is reliable on Windows where curl + SSE proved lossy.
var (
	logSink   *os.File
	logSinkMu sync.Mutex
)

func writeLogSink(line string) {
	if logSink == nil {
		return
	}
	logSinkMu.Lock()
	defer logSinkMu.Unlock()
	fmt.Fprintln(logSink, line)
}

// broadcaster fans out log lines to all connected SSE clients and keeps a
// small history so a freshly connected browser sees recent context.
type broadcaster struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
	history []string
}

func newBroadcaster() *broadcaster {
	return &broadcaster{clients: make(map[chan string]struct{})}
}

func (b *broadcaster) subscribe() (chan string, []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, 256)
	b.clients[ch] = struct{}{}
	return ch, append([]string(nil), b.history...)
}

func (b *broadcaster) unsubscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
}

func (b *broadcaster) publish(line string) {
	line = strings.ReplaceAll(line, "\r", "")
	writeLogSink(line)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.history = append(b.history, line)
	if len(b.history) > 500 {
		b.history = b.history[len(b.history)-500:]
	}
	for ch := range b.clients {
		select {
		case ch <- line:
		default: // drop for slow clients rather than block the reader
		}
	}
}

func main() {
	addr := flag.String("addr", ":8099", "HTTP listen address")
	name := flag.String("cmd", "", "subprocess command to launch")
	dir := flag.String("dir", "", "working directory for the subprocess (e.g. the Headless folder)")
	logfile := flag.String("logfile", "", "if set, write every published line verbatim to this file (empirical capture)")
	flag.Parse()
	args := flag.Args()

	if *logfile != "" {
		f, err := os.Create(*logfile)
		if err != nil {
			log.Fatalf("open logfile: %v", err)
		}
		logSink = f
	}

	if *name == "" {
		log.Fatal("usage: poc -cmd <command> [-addr :8099] [-- <args...>]")
	}

	bc := newBroadcaster()

	cmd := exec.Command(*name, args...)
	if *dir != "" {
		cmd.Dir = *dir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to start %q: %v", *name, err)
	}
	bc.publish(fmt.Sprintf("[poc] launched: %s %s (pid %d)", *name, strings.Join(args, " "), cmd.Process.Pid))

	// Read stdout/stderr as UTF-8 (correct for the Linux headless). The
	// Windows code page case will be handled later via x/text in v1.
	readPipe := func(r io.Reader, tag string) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			bc.publish(tag + sc.Text())
		}
	}
	go readPipe(stdout, "")
	go readPipe(stderr, "[stderr] ")
	go func() {
		err := cmd.Wait()
		bc.publish(fmt.Sprintf("[poc] process exited (%v)", err))
	}()

	index, _ := webFS.ReadFile("web/index.html")

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		ch, history := bc.subscribe()
		defer bc.unsubscribe(ch)
		for _, l := range history {
			fmt.Fprintf(w, "data: %s\n\n", l)
		}
		fl.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case line, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", line)
				fl.Flush()
			}
		}
	})
	// /quit: empirical-capture helper — let the script gracefully end the PoC so the
	// SSE connection closes naturally and curl flushes its file buffer on exit.
	mux.HandleFunc("/quit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		go func() {
			time.Sleep(150 * time.Millisecond)
			if logSink != nil {
				logSinkMu.Lock()
				_ = logSink.Sync()
				_ = logSink.Close()
				logSinkMu.Unlock()
			}
			os.Exit(0)
		}()
	})
	mux.HandleFunc("/command", func(w http.ResponseWriter, r *http.Request) {
		// Accept cmd from either query string or POST form body.
		c := r.URL.Query().Get("cmd")
		if c == "" {
			_ = r.ParseForm()
			c = r.FormValue("cmd")
		}
		if c == "" {
			http.Error(w, "missing cmd", http.StatusBadRequest)
			return
		}
		bc.publish("> " + c)
		if _, err := io.WriteString(stdin, c+"\n"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	bc.publish(fmt.Sprintf("[poc] HTTP listening on %s", *addr))
	log.Printf("PoC listening on %s — open http://localhost%s", *addr, *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
