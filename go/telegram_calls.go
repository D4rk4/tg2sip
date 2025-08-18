package main

import (
	client "github.com/zelenin/go-tdlib/client"
	"tg2sip/tgvoip"
)

// createTelegramCall starts a Telegram call to the specified user.
func createTelegramCall(cl *client.Client, userID int64) error {
	protocol := &client.CallProtocol{UdpP2p: true, UdpReflector: true, MinLayer: 65, MaxLayer: 92}
	if _, err := cl.CreateCall(&client.CreateCallRequest{UserId: userID, Protocol: protocol}); err != nil {
		return err
	}

	ctrl := tgvoip.NewController()
	key := make([]byte, 256)
	eps := []tgvoip.Endpoint{{ID: 1, IPv4: "0.0.0.0", Port: 0}}
	opts := tgvoip.DSPOptions{EchoCancellation: true, NoiseSuppression: true, AutoGain: true}
	_ = ctrl.Configure(key, eps, opts)
	ctrl.SetAudioCallbacks(func([]int16) {}, func([]int16) {})
	ctrl.Close()
	return nil
}

// acceptTelegramCall accepts an incoming Telegram call.
func acceptTelegramCall(cl *client.Client, callID int64) error {
	protocol := &client.CallProtocol{UdpP2p: true, UdpReflector: true, MinLayer: 65, MaxLayer: 92}
	if _, err := cl.AcceptCall(&client.AcceptCallRequest{CallId: callID, Protocol: protocol}); err != nil {
		return err
	}

	ctrl := tgvoip.NewController()
	key := make([]byte, 256)
	eps := []tgvoip.Endpoint{{ID: 1, IPv4: "0.0.0.0", Port: 0}}
	opts := tgvoip.DSPOptions{EchoCancellation: true, NoiseSuppression: true, AutoGain: true}
	_ = ctrl.Configure(key, eps, opts)
	ctrl.SetAudioCallbacks(func([]int16) {}, func([]int16) {})
	ctrl.Close()
	return nil
}

// discardTelegramCall terminates an ongoing Telegram call.
func discardTelegramCall(cl *client.Client, callID int64) error {
	_, err := cl.DiscardCall(&client.DiscardCallRequest{
		CallId:         int32(callID),
		IsDisconnected: false,
		Duration:       0,
		IsVideo:        false,
		ConnectionId:   client.JsonInt64(callID),
	})
	return err
}
