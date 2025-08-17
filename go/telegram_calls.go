package main

import client "github.com/zelenin/go-tdlib/client"

// createTelegramCall starts a Telegram call to the specified user.
func createTelegramCall(cl *client.Client, userID int64) error {
	protocol := &client.CallProtocol{UdpP2p: true, UdpReflector: true, MinLayer: 65, MaxLayer: 92}
	_, err := cl.CreateCall(&client.CreateCallRequest{UserId: userID, Protocol: protocol})
	return err
}

// acceptTelegramCall accepts an incoming Telegram call.
func acceptTelegramCall(cl *client.Client, callID int64) error {
	protocol := &client.CallProtocol{UdpP2p: true, UdpReflector: true, MinLayer: 65, MaxLayer: 92}
	_, err := cl.AcceptCall(&client.AcceptCallRequest{CallId: callID, Protocol: protocol})
	return err
}
