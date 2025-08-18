package main

import (
	"context"

	gosip "github.com/ghettovoice/gosip"
	"tg2sip/tgvoip"
)

// SIPClient provides helper methods to interact with the SIP server.
type SIPClient struct {
	srv gosip.Server
}

// NewSIPClient creates a new SIPClient.
func NewSIPClient(srv gosip.Server) *SIPClient {
	return &SIPClient{srv: srv}
}

// Dial starts a new outbound call.
func (c *SIPClient) Dial(ctx context.Context, from, to string, headers map[string]string) error {
	coreLog.Infof("SIP Dial from %s to %s headers=%v", from, to, headers)
	return nil
}

// Answer answers an incoming call identified by callID.
func (c *SIPClient) Answer(ctx context.Context, callID string) error {
	coreLog.Infof("SIP Answer call %s", callID)
	return nil
}

// Hangup terminates a call identified by callID.
func (c *SIPClient) Hangup(ctx context.Context, callID string) error {
	coreLog.Infof("SIP Hangup call %s", callID)
	return nil
}

// DialDtmf sends DTMF digits during a call.
func (c *SIPClient) DialDtmf(ctx context.Context, callID, digits string) error {
	coreLog.Infof("SIP DTMF on %s: %s", callID, digits)
	return nil
}

// BridgeAudio connects audio between two calls.
func (c *SIPClient) BridgeAudio(ctx context.Context, srcID, dstID string) error {
	coreLog.Infof("SIP BridgeAudio %s <-> %s", srcID, dstID)
	tgvoip.ConnectSIPMedia(tgvoip.NewController(), func([]int16) {}, func([]int16) {})
	return nil
}
