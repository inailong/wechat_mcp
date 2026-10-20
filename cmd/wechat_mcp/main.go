package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/nowa277/wechat_mcp/pkg/brain"
	"github.com/nowa277/wechat_mcp/pkg/bridge"
	"github.com/nowa277/wechat_mcp/pkg/daemon"
	"github.com/nowa277/wechat_mcp/pkg/daemon/config"
	"github.com/nowa277/wechat_mcp/pkg/dashboard"
	"github.com/nowa277/wechat_mcp/pkg/mcp"
	"github.com/nowa277/wechat_mcp/pkg/prompt"
	"github.com/nowa277/wechat_mcp/pkg/wechat"
)

func main() {
	var (
		sanitizeFlag = flag.Bool("sanitize", false, "Enable PII masking (phones, emails, IDs, bank cards)")
		doctorFlag   = flag.Bool("doctor", false, "Run diagnostic checks on WeChat native environment")
		installFlag  = flag.String("install", "", "Generate MCP configuration for client (claude-desktop, cursor, raw)")
		portFlag     = flag.Int("port", 0, "Custom HTTP fallback port")
		tokenFlag    = flag.String("token", "", "Custom HTTP fallback token")
		execFlag     = flag.String("executable", "", "Custom executable path for client configuration")

		// Daemon & Dashboard flags
		daemonFlag   = flag.Bool("daemon", false, "Run auto-reply daemon and web dashboard")
		webPortFlag  = flag.Int("web-port", 18080, "Port for web dashboard (default 18080)")
		brainURLFlag = flag.String("brain-url", "https://cpa.inailong.com/v1", "Nailong / OpenAI-compatible API base URL")
		brainKeyFlag = flag.String("brain-key", "sk-cliproxyapi-local-secure-key-9f8e7d6c", "API key for Brain client")
		brainModFlag = flag.String("brain-model", "gemini-3.8-flash-high", "Model name for Brain client")
	)

	flag.Parse()

	// 1. Doctor command
	if *doctorFlag {
		runDoctor(*portFlag, *tokenFlag)
		return
	}

	// 2. Install command
	if *installFlag != "" {
		runInstall(*installFlag, *execFlag, *sanitizeFlag)
		return
	}

	// 3. Daemon command
	if *daemonFlag {
		runDaemon(*webPortFlag, *brainURLFlag, *brainKeyFlag, *brainModFlag, *sanitizeFlag)
		return
	}

	// 4. Default: run MCP stdio server
	baseURL := ""
	if *portFlag > 0 {
		baseURL = fmt.Sprintf("http://127.0.0.1:%d", *portFlag)
	}

	client, err := bridge.NewClient(bridge.ClientOptions{
		BaseURL:  baseURL,
		Token:    *tokenFlag,
		Sanitize: *sanitizeFlag,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[wechat_mcp] Failed to initialize native engine: %v\n", err)
		os.Exit(1)
	}

	server := mcp.NewServer(client)
	if err := server.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "[wechat_mcp] Server error: %v\n", err)
		os.Exit(1)
	}
}

func runDoctor(customPort int, customToken string) {
	fmt.Println("\n[wechat_mcp doctor] WeChat MCP Standalone Diagnostics (Pure Go)")
	fmt.Println("---------------------------------------------------------------")

	// 1. Detect WeChat account
	info, err := wechat.DetectAccount()
	if err != nil {
		fmt.Printf("WeChat Account:      [FAIL] %v\n", err)
	} else {
		fmt.Printf("WeChat Account:      [OK] Detected\n")
		fmt.Printf("  WxID:              %s\n", info.WxID)
		if info.Nickname != "" {
			fmt.Printf("  Nickname:          %s\n", info.Nickname)
		}
		fmt.Printf("  Data Directory:    %s\n", info.DataDir)
		if info.SessionDb != "" {
			fmt.Printf("  Session DB:        %s\n", info.SessionDb)
		}
		if info.DecryptKey != "" {
			fmt.Printf("  Database Key:      [OK] Loaded (%s...)\n", info.DecryptKey[:8])
		}
	}

	// 2. Initialize Direct Native Engine
	client, err := bridge.NewClient(bridge.ClientOptions{})
	if err != nil {
		fmt.Printf("WCDB Direct Engine:  [FAIL] %v\n", err)
		fmt.Printf("\nVerdict: UNHEALTHY\n\n")
		os.Exit(1)
	}

	fmt.Printf("WCDB Direct Engine:  [OK] Native CGO Driver Active (Zero Electron)\n")

	// 3. Test read sessions
	ctx := context.Background()
	sessions, err := client.ListSessions(ctx, "", 3, 0)
	if err != nil {
		fmt.Printf("Chat DB Query:       [FAIL] %v\n", err)
		fmt.Printf("\nVerdict: UNHEALTHY\n\n")
		os.Exit(1)
	}

	fmt.Printf("Chat DB Query:       [OK] Direct read verified\n")
	fmt.Printf("  Active Sessions:   %d returned\n", len(sessions.Sessions))
	if len(sessions.Sessions) > 0 {
		fmt.Printf("  Latest Session:    %s (%s)\n", sessions.Sessions[0].DisplayName, sessions.Sessions[0].SessionID)
	}

	fmt.Printf("\nVerdict: HEALTHY (Standalone native engine ready, completely decoupled)\n\n")
}

func runInstall(client, customExec string, sanitize bool) {
	executable := customExec
	if executable == "" {
		if exe, err := os.Executable(); err == nil {
			executable = exe
		} else {
			executable = "wechat_mcp"
		}
	}
	executable = filepath.Clean(executable)

	args := []string{}
	if sanitize {
		args = append(args, "--sanitize")
	}

	var output interface{}
	switch client {
	case "claude-desktop", "cursor":
		output = map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"wechat": map[string]interface{}{
					"command": executable,
					"args":    args,
				},
			},
		}
	case "raw":
		output = map[string]interface{}{
			"command": executable,
			"args":    args,
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown client %q. Supported: claude-desktop, cursor, raw\n", client)
		os.Exit(2)
	}

	b, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(b))
}

func runDaemon(webPort int, brainURL, brainKey, brainModel string, sanitize bool) {
	fmt.Println("\n[wechat_mcp daemon] 微信智能自动代答中枢 (WeChat Auto-Reply Daemon)")
	fmt.Println("-----------------------------------------------------------------")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\n[wechat_mcp daemon] Received shutdown signal (%v), exiting cleanly...\n", sig)
		cancel()
	}()

	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "wechat_mcp")
	cfgPath := filepath.Join(configDir, "daemon_config.json")
	promptDir := filepath.Join(configDir, "prompts")

	// 1. Config & Prompts
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading daemon config: %v\n", err)
		os.Exit(1)
	}
	if webPort > 0 {
		cfg.Port = webPort
	}

	pStore, err := prompt.NewStore(promptDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing prompt store: %v\n", err)
		os.Exit(1)
	}

	// 2. Native Bridge Client
	bClient, err := bridge.NewClient(bridge.ClientOptions{
		Sanitize: sanitize,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing native bridge: %v\n", err)
		os.Exit(1)
	}

	// 3. Brain Client (Nailong API)
	brainClient := brain.NewClient(brainURL, brainKey, brainModel)

	// 4. Web Dashboard
	dashServer := dashboard.NewServer(cfg, pStore, bClient, cfg.Port)

	// 5. Auto-reply Daemon
	d := daemon.NewDaemon(cfg, pStore, brainClient, bClient, daemon.WithDashboard(dashServer))

	fmt.Printf("• 控制面板地址:  http://127.0.0.1:%d\n", cfg.Port)
	fmt.Printf("• 专属人设目录:  %s\n", promptDir)
	fmt.Printf("• 神思模型中枢:  %s (%s)\n", brainModel, brainURL)
	fmt.Printf("• 本地守护进程:  正在监听增量消息与光学死锁门禁...\n\n")

	go func() {
		if err := dashServer.Start(ctx); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "[dashboard] error: %v\n", err)
		}
	}()

	if err := d.Start(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "[daemon] error: %v\n", err)
	}
}

