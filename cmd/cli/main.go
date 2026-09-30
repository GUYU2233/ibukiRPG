// Command cli 是 ibukiRPG 的命令行调试入口（REPL 骨架）。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
)

func main() {
	showVersion := flag.Bool("version", false, "打印版本并退出")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.String())
		return
	}
	if err := repl(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func repl(in io.Reader, out io.Writer) error {
	fmt.Fprintf(out, "%s — 输入 /help 查看命令，/quit 退出\n", buildinfo.String())
	sc := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "> ")
		if !sc.Scan() {
			fmt.Fprintln(out)
			return sc.Err()
		}
		line := strings.TrimSpace(sc.Text())
		switch line {
		case "":
			continue
		case "/quit", "/exit":
			fmt.Fprintln(out, "再见。")
			return nil
		case "/help":
			fmt.Fprintln(out, "/help 帮助  /version 版本  /quit 退出；其他输入将作为玩家意图（尚未接入 Resolver）")
		case "/version":
			fmt.Fprintln(out, buildinfo.String())
		default:
			fmt.Fprintf(out, "[stub] 收到意图：%q（Resolver → 判定 → Event → Narrator → Guard 尚未实现）\n", line)
		}
	}
}
