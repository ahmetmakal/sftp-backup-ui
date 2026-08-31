package main

import (
	"fmt"
	"html/template"
	"log"
	"time"

	"github.com/ahmetmakal/sftp-backup-ui/internal/config"
	"github.com/ahmetmakal/sftp-backup-ui/internal/handlers"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	router := gin.Default()
	router.SetFuncMap(template.FuncMap{
		"gb": func(bytes uint64) string {
			return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
		},
		"pct": func(used, total uint64) int {
			if total == 0 {
				return 0
			}
			return int(min(used*100/total, 100))
		},
		"sub": func(a, b uint64) uint64 {
			if b > a {
				return 0
			}
			return a - b
		},
		"date": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("02.01.2006 15:04")
		},
		"daysSince": func(t time.Time) int {
			d := int(time.Since(t).Hours() / 24)
			if d < 0 {
				return 0
			}
			return d
		},
	})
	router.LoadHTMLGlob("templates/*.html")
	router.Static("/static", "./static")

	authorized := router.Group("/", gin.BasicAuth(gin.Accounts{
		cfg.AdminUser: cfg.AdminPassword,
	}))

	h := handlers.New(cfg)
	authorized.GET("/", h.Dashboard)
	authorized.GET("/users/new", h.NewUserForm)
	authorized.POST("/users", h.CreateUser)
	authorized.POST("/users/:username/quota", h.UpdateQuota)
	authorized.POST("/users/:username/ssh-key", h.UpdateSSHKey)
	authorized.POST("/users/:username/sftp", h.EnableSFTP)
	authorized.POST("/users/:username/rsync", h.EnableRsync)
	authorized.POST("/users/:username/rsync/disable", h.DisableRsync)
	authorized.POST("/users/:username/delete", h.DeleteUser)

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := router.Run(cfg.ListenAddr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
