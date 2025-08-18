package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	gosip "github.com/ghettovoice/gosip"
	gosiplog "github.com/ghettovoice/gosip/log"
	client "github.com/zelenin/go-tdlib/client"
	"gopkg.in/ini.v1"
)

var sipServer gosip.Server

func startSIP(cfg *Settings) error {
	coreLog.Info("starting SIP server")

	port := cfg.SIPPort()
	portRange := cfg.SIPPortRange()
	host := cfg.PublicAddress()

	logger := gosiplog.NewLogrusLogger(pjsipLog, "SIP", nil)

	sipServer = gosip.NewServer(gosip.ServerConfig{Host: host, UserAgent: "tg2sip"}, nil, nil, logger)

	var listenErr error
	for i := 0; i <= portRange; i++ {
		addr := fmt.Sprintf(":%d", port+i)
		listenErr = sipServer.Listen("udp", addr)
		if listenErr == nil {
			coreLog.Infof("SIP server listening on %s/udp", addr)
			return nil
		}
		coreLog.Warnf("failed to listen on %s: %v", addr, listenErr)
	}
	return fmt.Errorf("sip listen: %w", listenErr)
}

var tgClient *client.Client

func startTG(cfg *Settings) error {
	coreLog.Info("starting Telegram client")

	apiID := cfg.APIID()
	apiHash := cfg.APIHash()

	dataDir := cfg.DatabaseFolder()
	if dataDir == "" {
		dataDir = filepath.Join(".tdlib")
	}

	params := &client.SetTdlibParametersRequest{
		UseTestDc:              false,
		DatabaseDirectory:      filepath.Join(dataDir, "database"),
		FilesDirectory:         filepath.Join(dataDir, "files"),
		UseFileDatabase:        true,
		UseChatInfoDatabase:    true,
		UseMessageDatabase:     true,
		UseSecretChats:         false,
		ApiId:                  int32(apiID),
		ApiHash:                apiHash,
		SystemLanguageCode:     cfg.SystemLanguageCode(),
		DeviceModel:            cfg.DeviceModel(),
		SystemVersion:          cfg.SystemVersion(),
		ApplicationVersion:     cfg.ApplicationVersion(),
		EnableStorageOptimizer: true,
	}

	authorizer := client.ClientAuthorizer(params)
	go client.CliInteractor(authorizer)

	var err error
	tgClient, err = client.NewClient(authorizer)
	if err != nil {
		return fmt.Errorf("tdlib client: %w", err)
	}

	if cfg.ProxyEnabled() {
		proxyAddr := cfg.ProxyAddress()
		proxyPort := cfg.ProxyPort()
		if proxyAddr != "" && proxyPort != 0 {
			ptype := &client.ProxyTypeSocks5{
				Username: cfg.ProxyUsername(),
				Password: cfg.ProxyPassword(),
			}
			_, err := tgClient.AddProxy(&client.AddProxyRequest{
				Server: proxyAddr,
				Port:   int32(proxyPort),
				Enable: true,
				Type:   ptype,
			})
			if err != nil {
				return fmt.Errorf("telegram.add_proxy: %w", err)
			}
		} else {
			coreLog.Warn("telegram proxy enabled but address or port missing")
		}
	}

	me, err := tgClient.GetMe(context.Background())
	if err != nil {
		return fmt.Errorf("get me: %w", err)
	}
	coreLog.Infof("telegram authorized as %s %s (@%s)", me.FirstName, me.LastName, me.Username)
	return nil
}

func main() {
	cfg, err := ini.Load("../settings.ini")
	if err != nil {
		fmt.Printf("failed to load settings: %v\n", err)
		return
	}

	settings, err := LoadSettings(cfg)
	if err != nil {
		fmt.Printf("failed to parse settings: %v\n", err)
		return
	}

	if err := initLogging(cfg); err != nil {
		fmt.Printf("failed to init logging: %v\n", err)
		return
	}
	coreLog.Info("settings loaded", cfg.Section("").KeysHash())

	if err := startSIP(settings); err != nil {
		coreLog.Fatalf("failed to start SIP client: %v", err)
	}
	if err := startTG(settings); err != nil {
		coreLog.Fatalf("failed to start Telegram client: %v", err)
	}
	if err := startGateway(settings); err != nil {
		coreLog.Fatalf("failed to start gateway: %v", err)
	}

	coreLog.Info("performing a graceful shutdown...")
	time.Sleep(time.Second)
}
