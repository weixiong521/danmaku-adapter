package main

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// adminBody 是 /admin 系列接口的请求体。
type adminBody struct {
	ResourceHosts  []string `json:"resource_hosts"`
	SourcePriority []string `json:"source_priority"`
	LogVarBase     string   `json:"logvar_base"`
	LogVarToken    string   `json:"logvar_token"`
}

func (s *Server) registerAdmin(mux *http.ServeMux) {
	mux.HandleFunc("/admin/", s.handleAdmin)
}

// adminAuthorized 校验管理接口访问权限：
//   - 未开启 admin：一律拒绝
//   - 未设置 token：放行（建议仅在受控内网使用）
//   - 设置了 token：需在请求头 X-Admin-Token 或 ?token= 中携带正确令牌
func (s *Server) adminAuthorized(r *http.Request) bool {
	cfg := s.store.Get()
	if !cfg.AdminEnabled {
		return false
	}
	if cfg.AdminToken == "" {
		return true
	}
	tok := r.Header.Get("X-Admin-Token")
	if tok == "" {
		tok = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(tok), []byte(cfg.AdminToken)) == 1
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		http.Error(w, "unauthorized: 管理接口未开启或令牌错误", http.StatusUnauthorized)
		return
	}
	switch strings.TrimRight(r.URL.Path, "/") {
	case "/admin/sources":
		s.adminSources(w, r)
	case "/admin/priority":
		s.adminPriority(w, r)
	case "/admin/logvar":
		s.adminLogvar(w, r)
	case "/admin/reload":
		s.adminReload(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) readAdminBody(w http.ResponseWriter, r *http.Request) (*adminBody, bool) {
	b := &adminBody{}
	if r.Body != nil {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		if err := dec.Decode(b); err != nil && err != io.EOF {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return nil, false
		}
	}
	// 兼容 DELETE / 部分客户端不带 body：支持 ?host=a&host=b
	if len(b.ResourceHosts) == 0 {
		for _, h := range r.URL.Query()["host"] {
			if t := strings.TrimSpace(h); t != "" {
				b.ResourceHosts = append(b.ResourceHosts, t)
			}
		}
	}
	return b, true
}

// /admin/sources
//
//	GET    -> 查看当前资源站
//	POST   -> 新增（追加）
//	PUT    -> 整体替换
//	DELETE -> 删除
func (s *Server) adminSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeJSON(w, map[string]any{"code": 1, "data": s.sm.Snapshot()})

	case http.MethodPost:
		b, ok := s.readAdminBody(w, r)
		if !ok {
			return
		}
		if len(b.ResourceHosts) == 0 {
			http.Error(w, "resource_hosts 不能为空", http.StatusBadRequest)
			return
		}
		added, cur := s.sm.AddResourceHosts(b.ResourceHosts)
		s.writeJSON(w, map[string]any{"code": 1, "added": added, "resource_hosts": cur, "count": len(cur)})

	case http.MethodPut:
		b, ok := s.readAdminBody(w, r)
		if !ok {
			return
		}
		cur := s.sm.SetResourceHosts(b.ResourceHosts)
		s.writeJSON(w, map[string]any{"code": 1, "resource_hosts": cur, "count": len(cur)})

	case http.MethodDelete:
		b, ok := s.readAdminBody(w, r)
		if !ok {
			return
		}
		if len(b.ResourceHosts) == 0 {
			http.Error(w, "resource_hosts 不能为空", http.StatusBadRequest)
			return
		}
		removed, cur := s.sm.RemoveResourceHosts(b.ResourceHosts)
		s.writeJSON(w, map[string]any{"code": 1, "removed": removed, "resource_hosts": cur, "count": len(cur)})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// POST /admin/priority  {"source_priority":["tencent","iqiyi",...]}
func (s *Server) adminPriority(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b, ok := s.readAdminBody(w, r)
	if !ok {
		return
	}
	if len(b.SourcePriority) == 0 {
		http.Error(w, "source_priority 不能为空", http.StatusBadRequest)
		return
	}
	cur := s.sm.SetSourcePriority(b.SourcePriority)
	s.writeJSON(w, map[string]any{"code": 1, "source_priority": cur})
}

// POST /admin/logvar  {"logvar_base":"https://...","logvar_token":"xxx"}
func (s *Server) adminLogvar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b, ok := s.readAdminBody(w, r)
	if !ok {
		return
	}
	if b.LogVarBase == "" && b.LogVarToken == "" {
		http.Error(w, "logvar_base / logvar_token 至少提供一个", http.StatusBadRequest)
		return
	}
	s.sm.SetLogVar(b.LogVarBase, b.LogVarToken)
	cfg := s.store.Get()
	s.writeJSON(w, map[string]any{"code": 1, "logvar_base": cfg.LogVarBase, "logvar_token": maskToken(cfg.LogVarToken)})
}

// POST /admin/reload  重新读取 config.json + 环境变量
func (s *Server) adminReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := s.store.Reload()
	s.writeJSON(w, map[string]any{
		"code":            1,
		"reloaded":        true,
		"resource_hosts":  cfg.ResourceHosts,
		"source_priority": cfg.SourcePriority,
	})
}
