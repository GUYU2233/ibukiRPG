package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/GUYU2233/ibukiRPG/internal/adapter/mcp"
	adapter "github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
	"github.com/GUYU2233/ibukiRPG/packages"
)

// runMCP 实现 `ibukirpg mcp`：以 stdio 运行只读 MCP 服务器（第 48 节）。
// stdout 只输出协议消息，日志一律写 stderr。
func runMCP(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	save := fs.String("save", "", "存档数据库文件（或存档目录，默认 "+filepath.Join(defaultDataDir(), adapter.DBFileName)+"）")
	slot := fs.String("slot", "", "存档 ID（留空为最近更新的存档）")
	scope := fs.String("scope", "player", "检索范围：player（叙述者 / 玩家已知）/ director（完整设定）/ npc:<角色ID>")
	packs := fs.String("packs", "", "导入故事包目录（默认为存档目录下的 packs/）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sc, err := tools.ParseScope(*scope)
	if err != nil {
		return err
	}
	path := *save
	if path == "" {
		path = filepath.Join(defaultDataDir(), adapter.DBFileName)
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, adapter.DBFileName)
	} else if err != nil {
		return fmt.Errorf("找不到存档数据库 %s：%w", path, err)
	}
	packDir := *packs
	if packDir == "" {
		packDir = filepath.Join(filepath.Dir(path), adapter.PackDirName)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	s, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: path, Packs: packages.Builtin(), PackDir: packDir})
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	// 启动时检查一次，尽早报错（例如没有任何存档）
	_, id, err := s.SlotToolEnv(ctx, *slot)
	if errors.Is(err, orchestrator.ErrNoGame) {
		return fmt.Errorf("%s 里还没有存档", path)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "ibukirpg mcp: save=%s slot=%s scope=%s（只读）\n", path, id, sc)
	srv := &mcp.Server{Scope: sc, Name: "ibukirpg", Version: buildinfo.Version,
		Env: func(ctx context.Context) (*tools.Env, error) {
			env, _, err := s.SlotToolEnv(ctx, *slot)
			return env, err
		}}
	return srv.Serve(ctx, stdin, stdout)
}
