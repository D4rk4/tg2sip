package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	gosip "github.com/ghettovoice/gosip"
	gosiplog "github.com/ghettovoice/gosip/log"
	client "github.com/zelenin/go-tdlib/client"
	"gopkg.in/ini.v1"
)

var sipServer gosip.Server

func startSIP(ctx context.Context, cfg *Settings) error {
	coreLog.Info("starting SIP server")

	port := cfg.SIPPort()
	portRange := cfg.SIPPortRange()
	host := cfg.PublicAddress()
	if host == "" {
		if ip, err := detectHostIP(); err != nil {
			coreLog.Warnf("auto detect address: %v", err)
		} else {
			host = ip
			coreLog.Infof("auto detected address %s", host)
		}
	}

	logger := gosiplog.NewLogrusLogger(pjsipLog, "SIP", nil)

	sipServer = gosip.NewServer(gosip.ServerConfig{Host: host, UserAgent: "tg2sip"}, nil, nil, logger)

	var listenErr error
	for i := 0; i <= portRange; i++ {
		// Explicitly include host in listen address to avoid binding to
		// the default loopback address when a public address is
		// detected. When host is empty, fall back to all interfaces.
		addr := fmt.Sprintf(":%d", port+i)
		if host != "" {
			addr = fmt.Sprintf("%s:%d", host, port+i)
		}
		listenErr = sipServer.Listen("udp", addr)
		if listenErr == nil {
			coreLog.Infof("SIP server listening on %s/udp", addr)
			go func() {
				<-ctx.Done()
				sipServer.Shutdown()
			}()
			return nil
		}
		coreLog.Warnf("failed to listen on %s: %v", addr, listenErr)
	}
	return fmt.Errorf("sip listen: %w", listenErr)
}

var tgClient *client.Client

func startTG(ctx context.Context, cfg *Settings) error {
	coreLog.Info("starting Telegram client")

	apiID := cfg.APIID()
	apiHash := cfg.APIHash()

	dataDir := cfg.DatabaseFolder()
	if dataDir == "" {
		dataDir = filepath.Join(".tdlib")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	params := &client.SetTdlibParametersRequest{
		UseTestDc:           false,
		DatabaseDirectory:   filepath.Join(dataDir, "database"),
		FilesDirectory:      filepath.Join(dataDir, "files"),
		UseFileDatabase:     true,
		UseChatInfoDatabase: true,
		UseMessageDatabase:  true,
		UseSecretChats:      false,
		ApiId:               int32(apiID),
		ApiHash:             apiHash,
		SystemLanguageCode:  cfg.SystemLanguageCode(),
		DeviceModel:         cfg.DeviceModel(),
		SystemVersion:       cfg.SystemVersion(),
		ApplicationVersion:  cfg.ApplicationVersion(),
	}

	authorizer := client.ClientAuthorizer(params)
	go func() {
		for state := range authorizer.State {
			switch state.AuthorizationStateType() {
			case client.TypeAuthorizationStateWaitPhoneNumber:
				phone := cfg.PhoneNumber()
				if phone == "" {
					fmt.Println("Enter phone number: ")
					fmt.Scanln(&phone)
				} else {
					fmt.Println("Using phone number from settings")
				}
				authorizer.PhoneNumber <- phone
			case client.TypeAuthorizationStateWaitCode:
				fmt.Println("Enter code: ")
				var code string
				fmt.Scanln(&code)
				authorizer.Code <- code
			case client.TypeAuthorizationStateWaitPassword:
				fmt.Println("Enter password: ")
				var password string
				fmt.Scanln(&password)
				authorizer.Password <- password
			case client.TypeAuthorizationStateReady:
				return
			}
		}
	}()

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

	me, err := tgClient.GetMe()
	if err != nil {
		return fmt.Errorf("get me: %w", err)
	}
	coreLog.Infof("telegram authorized as %s %s (@%s)", me.FirstName, me.LastName, getUsername(me))

	go func() {
		<-ctx.Done()
		if tgClient != nil {
			if _, err := tgClient.Close(); err != nil {
				coreLog.Warnf("telegram client close: %v", err)
			}
		}
	}()

	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	if err := startSIP(ctx, settings); err != nil {
		coreLog.Fatalf("failed to start SIP client: %v", err)
	}
	if err := startTG(ctx, settings); err != nil {
		coreLog.Fatalf("failed to start Telegram client: %v", err)
	}
	if err := startGateway(ctx, settings); err != nil {
		coreLog.Fatalf("failed to start gateway: %v", err)
	}

	coreLog.Info("performing a graceful shutdown...")
	time.Sleep(time.Second)
	closeLogging()
}
