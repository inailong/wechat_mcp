package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/inailong/wechat_mcp/pkg/bridge"
	"github.com/inailong/wechat_mcp/pkg/daemon/config"
	"github.com/inailong/wechat_mcp/pkg/prompt"
)

//go:embed ui/*
var uiFS embed.FS

type LogEntry struct {
	Time     string `json:"time"`
	Sender   string `json:"sender"`
	Incoming string `json:"incoming"`
	Reply    string `json:"reply"`
	Latency  string `json:"latency"`
}

type ContactItem struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Remark      string `json:"remark,omitempty"`
	Type        string `json:"type,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type Server struct {
	cfg         *config.DaemonConfig
	promptStore *prompt.Store
	bClient     *bridge.Client
	port        int
	logs        []LogEntry
	logMu       sync.RWMutex
}

func NewServer(cfg *config.DaemonConfig, promptStore *prompt.Store, bClient *bridge.Client, port int) *Server {
	if port == 0 && cfg != nil {
		port = cfg.Port
	}
	if port == 0 {
		port = 18080
	}
	return &Server{
		cfg:         cfg,
		promptStore: promptStore,
		bClient:     bClient,
		port:        port,
		logs:        make([]LogEntry, 0),
	}
}

func (s *Server) AddLog(sender, incoming, reply, latency string) {
	s.logMu.Lock()
	defer s.logMu.Unlock()

	entry := LogEntry{
		Time:     time.Now().Format("15:04:05"),
		Sender:   sender,
		Incoming: incoming,
		Reply:    reply,
		Latency:  latency,
	}

	s.logs = append([]LogEntry{entry}, s.logs...)
	if len(s.logs) > 50 {
		s.logs = s.logs[:50]
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 1. Static UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		data, err := uiFS.ReadFile("ui/index.html")
		if err != nil {
			http.Error(w, "embedded UI missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// 2. GET /api/contacts
	mux.HandleFunc("/api/contacts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var items []ContactItem
		if s.bClient != nil {
			sessions, err := s.bClient.ListSessions(r.Context(), "", 100, 0)
			if err == nil && sessions != nil {
				for _, sess := range sessions.Sessions {
					uname := sess.Username
					if uname == "" {
						uname = sess.SessionID
					}
					enabled := false
					if s.cfg != nil {
						enabled = s.cfg.IsEnabled(uname)
					}
					items = append(items, ContactItem{
						Username:    uname,
						DisplayName: sess.DisplayName,
						Type:        sess.SessionType,
						Enabled:     enabled,
					})
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"contacts": items,
		})
	})

	// 3. POST /api/toggle
	mux.HandleFunc("/api/toggle", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Username string `json:"username"`
			Enabled  bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if payload.Username == "" {
			http.Error(w, "username required", http.StatusBadRequest)
			return
		}

		if s.cfg != nil {
			if err := s.cfg.SetEnabled(payload.Username, payload.Enabled); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	})

	// 4. GET / POST /api/prompt
	mux.HandleFunc("/api/prompt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			username := r.URL.Query().Get("username")
			if username == "__default__" {
				username = ""
			}
			promptText, err := s.promptStore.GetPrompt(username)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"username": username,
				"prompt":   promptText,
			})
			return
		}

		if r.Method == http.MethodPost {
			var payload struct {
				Username string `json:"username"`
				Prompt   string `json:"prompt"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			payload.Username = strings.TrimSpace(payload.Username)
			if payload.Username == "" {
				http.Error(w, "username required", http.StatusBadRequest)
				return
			}

			if err := s.promptStore.SavePrompt(payload.Username, payload.Prompt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
			return
		}

		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// 5. GET /api/logs
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.logMu.RLock()
		defer s.logMu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logs": s.logs,
		})
	})

	return mux
}

func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}
