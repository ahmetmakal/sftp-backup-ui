// Package handlers wires the sysops package (which does everything and
// touches the real system) to Gin routes and html/template views.
package handlers

import "github.com/ahmetmakal/sftp-backup-ui/internal/config"

type Handlers struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Handlers {
	return &Handlers{cfg: cfg}
}
