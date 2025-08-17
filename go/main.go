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

func startSIP(cfg *ini.File) error {
	coreLog.Info("starting SIP server")
	sec := cfg.Section("sip")

	port := sec.Key("port").MustInt(5060)
	portRange := sec.Key("port_range").MustInt(0)
	host := sec.Key("public_address").String()

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

func startTG(cfg *ini.File) error {
	coreLog.Info("starting Telegram client")
	sec := cfg.Section("telegram")

	apiID, err := sec.Key("api_id").Int()
	if err != nil {
		return fmt.Errorf("telegram.api_id: %w", err)
	}
	apiHash := sec.Key("api_hash").String()
	if apiHash == "" {
		return fmt.Errorf("telegram.api_hash is empty")
	}

	dataDir := sec.Key("database_folder").String()
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
		SystemLanguageCode:     sec.Key("system_language_code").MustString("en"),
		DeviceModel:            sec.Key("device_model").MustString("tg2sip"),
		SystemVersion:          sec.Key("system_version").MustString("linux"),
		ApplicationVersion:     sec.Key("application_version").MustString("0.1"),
		EnableStorageOptimizer: true,
	}

	authorizer := client.ClientAuthorizer(params)
	go client.CliInteractor(authorizer)

	tgClient, err = client.NewClient(authorizer)
	if err != nil {
		return fmt.Errorf("tdlib client: %w", err)
	}

	if sec.Key("use_proxy").MustBool(false) {
		proxyAddr := sec.Key("proxy_address").String()
		proxyPort := sec.Key("proxy_port").MustInt(0)
		if proxyAddr != "" && proxyPort != 0 {
			ptype := &client.ProxyTypeSocks5{
				Username: sec.Key("proxy_username").String(),
				Password: sec.Key("proxy_password").String(),
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
	if err := initLogging(cfg); err != nil {
		fmt.Printf("failed to init logging: %v\n", err)
		return
	}
	coreLog.Info("settings loaded", cfg.Section("").KeysHash())

	if err := startSIP(cfg); err != nil {
		coreLog.Fatalf("failed to start SIP client: %v", err)
	}
	if err := startTG(cfg); err != nil {
		coreLog.Fatalf("failed to start Telegram client: %v", err)
	}
	if err := startGateway(); err != nil {
		coreLog.Fatalf("failed to start gateway: %v", err)
	}

	coreLog.Info("performing a graceful shutdown...")
	time.Sleep(time.Second)
}
