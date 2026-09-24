package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func nowUnix() int64 { return time.Now().Unix() }

func configPath() string {
	if v := env("CONFIG_PATH"); v != "" {
		return v
	}
	return "config.json"
}

func main() {
	store := NewConfigStore(configPath())
	cfg := store.Get()

	s := &Server{
		store: store,
		lv:    NewLogVarClient(store),
		jl:    NewJuliangResolver(store),
		sm:    NewSourceManager(store),
		cms:   NewCMSCrawler(store),
	}

	mux := http.NewServeMux()

	// Getapp 弹幕入口：根路径（?ac=dm）以及 /dm、/danmu、/cms 均可
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/dm", "/danmu", "/cms":
			s.handleDanmu(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write([]byte("ok"))
	})

	// 管理接口：资源站热更新（默认关闭，需 ADMIN_ENABLED=true）
	s.registerAdmin(mux)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 后台热重载 config.json
	go store.Watch(ctx)

	srv := &http.Server{Addr: cfg.Listen, Handler: mux}

	go func() {
		log.Printf("LogVar-Getapp 弹幕适配服务启动：监听 %s，LogVar=%s，资源站=%d 个，管理接口=%v",
			cfg.Listen, cfg.LogVarBase, len(cfg.ResourceHosts), cfg.AdminEnabled)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务异常退出：%v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("收到退出信号，正在优雅关闭…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭超时：%v", err)
	}
	log.Printf("服务已退出")
}
