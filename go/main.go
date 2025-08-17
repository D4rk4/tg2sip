package main

import (
	"log"
	"time"

	"gopkg.in/ini.v1"
)

func startSIP() error {
	log.Println("starting SIP client (stub)")
	return nil
}

func startTG() error {
	log.Println("starting Telegram client (stub)")
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
	if err := startTG(); err != nil {
		log.Fatalf("failed to start Telegram client: %v", err)
	}
	if err := startGateway(); err != nil {
		log.Fatalf("failed to start gateway: %v", err)
	}

	log.Println("performing a graceful shutdown...")
	time.Sleep(time.Second)
}
