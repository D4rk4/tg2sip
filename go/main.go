package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	client "github.com/zelenin/go-tdlib/client"
	"gopkg.in/ini.v1"
)

func startSIP() error {
	log.Println("starting SIP client (stub)")
	return nil
}

var tgClient *client.Client

func startTG(cfg *ini.File) error {
	log.Println("starting Telegram client")
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

	me, err := tgClient.GetMe(context.Background())
	if err != nil {
		return fmt.Errorf("get me: %w", err)
	}
	log.Printf("telegram authorized as %s %s (@%s)", me.FirstName, me.LastName, me.Username)
	return nil
}

func startGateway() error {
	log.Println("starting gateway (stub)")
	return nil
}

func main() {
	log.SetPrefix("tg2sip: ")

	cfg, err := ini.Load("../settings.ini")
	if err != nil {
		log.Fatalf("failed to load settings: %v", err)
	}
	log.Println("settings loaded", cfg.Section("").KeysHash())

	if err := startSIP(); err != nil {
		log.Fatalf("failed to start SIP client: %v", err)
	}
	if err := startTG(cfg); err != nil {
		log.Fatalf("failed to start Telegram client: %v", err)
	}
	if err := startGateway(); err != nil {
		log.Fatalf("failed to start gateway: %v", err)
	}

	log.Println("performing a graceful shutdown...")
	time.Sleep(time.Second)
}
