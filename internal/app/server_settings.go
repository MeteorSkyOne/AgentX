package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/meteorsky/agentx/internal/config"
)

type ServerSettings struct {
	OrganizationID     string            `json:"organization_id"`
	ListenIP           string            `json:"listen_ip"`
	ListenPort         int               `json:"listen_port"`
	AddrOverrideActive bool              `json:"addr_override_active"`
	AddrOverrideValue  string            `json:"addr_override_value,omitempty"`
	EffectiveAddr      string            `json:"effective_addr"`
	EffectiveHTTPAddr  string            `json:"effective_http_addr"`
	EffectiveHTTPSAddr string            `json:"effective_https_addr,omitempty"`
	RestartRequired    bool              `json:"restart_required"`
	TLS                ServerTLSSettings `json:"tls"`
}

type ServerTLSSettings struct {
	Enabled    bool   `json:"enabled"`
	ListenPort int    `json:"listen_port"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
}

type ServerSettingsUpdateRequest struct {
	ListenIP   string                         `json:"listen_ip"`
	ListenPort int                            `json:"listen_port"`
	TLS        ServerTLSSettingsUpdateRequest `json:"tls"`
}

type ServerTLSSettingsUpdateRequest struct {
	Enabled    bool    `json:"enabled"`
	ListenPort int     `json:"listen_port"`
	CertFile   string  `json:"cert_file"`
	KeyFile    string  `json:"key_file"`
	CertPEM    *string `json:"cert_pem,omitempty"`
	KeyPEM     *string `json:"key_pem,omitempty"`
}

func (a *App) ServerSettings(ctx context.Context, orgID string) (ServerSettings, error) {
	settings, err := config.LoadServerSettings(a.opts.DataDir)
	if err != nil {
		return ServerSettings{}, err
	}
	return a.serverSettingsResponse(orgID, settings), nil
}

func (a *App) UpdateServerSettings(ctx context.Context, orgID string, req ServerSettingsUpdateRequest) (ServerSettings, error) {
	certPEM := strings.TrimSpace(valueOrEmpty(req.TLS.CertPEM))
	keyPEM := strings.TrimSpace(valueOrEmpty(req.TLS.KeyPEM))
	settings := config.ServerSettings{
		ListenIP:   req.ListenIP,
		ListenPort: req.ListenPort,
		TLS: config.ServerTLSSettings{
			Enabled:    req.TLS.Enabled,
			ListenPort: req.TLS.ListenPort,
			CertFile:   req.TLS.CertFile,
			KeyFile:    req.TLS.KeyFile,
		},
	}

	certPath := filepath.Join(a.opts.DataDir, "cert")
	keyPath := filepath.Join(a.opts.DataDir, "privkey")
	if certPEM != "" {
		if err := validateServerPEM(certPEM, "certificate"); err != nil {
			return ServerSettings{}, err
		}
		settings.TLS.CertFile = certPath
	}
	if keyPEM != "" {
		if err := validateServerPEM(keyPEM, "private key"); err != nil {
			return ServerSettings{}, err
		}
		settings.TLS.KeyFile = keyPath
	}

	normalized, err := config.NormalizeServerSettings(settings, config.ConfigPath(a.opts.DataDir))
	if err != nil {
		return ServerSettings{}, invalidInput(err.Error())
	}
	if certPEM != "" {
		if err := writeServerPEMFile(certPath, certPEM, 0o644); err != nil {
			return ServerSettings{}, err
		}
	}
	if keyPEM != "" {
		if err := writeServerPEMFile(keyPath, keyPEM, 0o600); err != nil {
			return ServerSettings{}, err
		}
	}
	saved, err := config.SaveServerSettings(a.opts.DataDir, normalized)
	if err != nil {
		return ServerSettings{}, err
	}
	return a.serverSettingsResponse(orgID, saved), nil
}

func validateServerPEM(body string, label string) error {
	if !strings.Contains(body, "-----BEGIN ") || !strings.Contains(body, "-----END ") {
		return invalidInput(label + " must be PEM encoded")
	}
	return nil
}

func writeServerPEMFile(path string, body string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body += "\n"
	if err := os.WriteFile(path, []byte(body), perm); err != nil {
		return err
	}
	if err := os.Chmod(path, perm); err != nil {
		return err
	}
	return nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (a *App) serverSettingsResponse(orgID string, settings config.ServerSettings) ServerSettings {
	startup := a.opts.ServerSettings
	if startup.ListenIP == "" && startup.ListenPort == 0 {
		startup = config.DefaultServerSettings()
	}
	effectiveHTTPAddr := config.ServerAddr(settings)
	if strings.TrimSpace(a.opts.ServerAddr) != "" {
		effectiveHTTPAddr = a.opts.ServerAddr
	}
	effectiveHTTPSAddr := ""
	if settings.TLS.Enabled {
		effectiveHTTPSAddr = config.ServerTLSAddr(settings)
	}

	return ServerSettings{
		OrganizationID:     orgID,
		ListenIP:           settings.ListenIP,
		ListenPort:         settings.ListenPort,
		AddrOverrideActive: a.opts.AddrOverride,
		AddrOverrideValue:  a.opts.AddrOverrideValue,
		EffectiveAddr:      effectiveHTTPAddr,
		EffectiveHTTPAddr:  effectiveHTTPAddr,
		EffectiveHTTPSAddr: effectiveHTTPSAddr,
		RestartRequired:    !config.ServerSettingsEqual(settings, startup),
		TLS: ServerTLSSettings{
			Enabled:    settings.TLS.Enabled,
			ListenPort: settings.TLS.ListenPort,
			CertFile:   settings.TLS.CertFile,
			KeyFile:    settings.TLS.KeyFile,
		},
	}
}
