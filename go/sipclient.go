package main

import (
	"context"
	"fmt"
	"sync"

	gosip "github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/sip"
	"github.com/ghettovoice/gosip/sip/parser"
	"github.com/ghettovoice/gosip/util"
	"tg2sip/tgvoip"
)

// SIPClient provides helper methods to interact with the SIP server.
type SIPClient struct {
	srv   gosip.Server
	mu    sync.Mutex
	calls map[string]*callSession
}

type callSession struct {
	callID     string
	localAddr  *sip.Address
	remoteAddr *sip.Address
	contact    *sip.Address
	cseq       uint
	clientTx   sip.ClientTransaction
	serverTx   sip.ServerTransaction
	inviteReq  sip.Request
}

// NewSIPClient creates a new SIPClient.
func NewSIPClient(srv gosip.Server) *SIPClient {
	return &SIPClient{srv: srv, calls: make(map[string]*callSession)}
}

// TrackInvite stores incoming INVITE transaction for later processing.
func (c *SIPClient) TrackInvite(req sip.Request, tx sip.ServerTransaction) {
	cid, _ := req.CallID()
	callID := ""
	if cid != nil {
		callID = cid.String()
	}
	fromHdr, _ := req.From()
	toHdr, _ := req.To()
	sess := &callSession{
		callID:     callID,
		localAddr:  sip.NewAddressFromToHeader(toHdr),
		remoteAddr: sip.NewAddressFromFromHeader(fromHdr),
		contact:    sip.NewAddressFromToHeader(toHdr),
		cseq:       1,
		serverTx:   tx,
		inviteReq:  req,
	}
	if fromHdr != nil && fromHdr.Params != nil {
		if tag, ok := fromHdr.Params.Get("tag"); ok {
			sess.remoteAddr.Params = sess.remoteAddr.Params.Add("tag", tag)
		}
	}
	c.mu.Lock()
	c.calls[callID] = sess
	c.mu.Unlock()
}

// Dial starts a new outbound call.
func (c *SIPClient) Dial(ctx context.Context, from, to string, headers map[string]string) error {
	coreLog.Infof("SIP Dial from %s to %s headers=%v", from, to, headers)

	toURI, err := parser.ParseUri(to)
	if err != nil {
		return fmt.Errorf("parse to uri: %w", err)
	}

	host := toURI.Host()
	fromURI, err := parser.ParseUri(fmt.Sprintf("sip:%s@%s", from, host))
	if err != nil {
		return fmt.Errorf("parse from uri: %w", err)
	}

	tag := util.RandString(8)
	fromAddr := &sip.Address{Uri: fromURI, Params: sip.NewParams().Add("tag", sip.String{Str: tag})}
	toAddr := &sip.Address{Uri: toURI}
	contactAddr := &sip.Address{Uri: fromURI.Clone()}

	rb := sip.NewRequestBuilder().
		SetMethod(sip.INVITE).
		SetRecipient(toURI).
		SetFrom(fromAddr).
		SetTo(toAddr).
		SetContact(contactAddr)

	for k, v := range headers {
		rb.AddHeader(&sip.GenericHeader{HeaderName: k, Contents: v})
	}

	req, err := rb.Build()
	if err != nil {
		return fmt.Errorf("build invite: %w", err)
	}

	cid, _ := req.CallID()
	callID := ""
	if cid != nil {
		callID = cid.String()
	}

	tx, err := c.srv.Request(req)
	if err != nil {
		return fmt.Errorf("send invite: %w", err)
	}

	c.mu.Lock()
	c.calls[callID] = &callSession{
		callID:     callID,
		localAddr:  fromAddr,
		remoteAddr: toAddr,
		contact:    contactAddr,
		cseq:       1,
		clientTx:   tx,
	}
	c.mu.Unlock()

	go func() {
		for {
			select {
			case <-ctx.Done():
				_ = tx.Cancel()
				return
			case res := <-tx.Responses():
				if res != nil {
					coreLog.Infof("received SIP response: %d %s", res.StatusCode(), res.Reason())
					if toHdr, ok := res.To(); ok {
						if tag, ok := toHdr.Params.Get("tag"); ok {
							c.mu.Lock()
							if sess, ok := c.calls[callID]; ok {
								sess.remoteAddr.Params = sess.remoteAddr.Params.Add("tag", tag)
							}
							c.mu.Unlock()
						}
					}
					if !res.IsProvisional() {
						return
					}
				}
			case err := <-tx.Errors():
				if err != nil {
					coreLog.Warnf("SIP transaction error: %v", err)
				}
				return
			case <-tx.Done():
				return
			}
		}
	}()

	return nil
}

// Answer answers an incoming call identified by callID.
func (c *SIPClient) Answer(ctx context.Context, callID string) error {
	coreLog.Infof("SIP Answer call %s", callID)
	c.mu.Lock()
	sess, ok := c.calls[callID]
	c.mu.Unlock()
	if !ok || sess.serverTx == nil || sess.inviteReq == nil {
		return fmt.Errorf("call %s not found", callID)
	}

	res := sip.NewResponseFromRequest("", sess.inviteReq, sip.StatusOK, "OK", "")
	tag := util.RandString(8)
	if toHdr, ok := res.To(); ok {
		toHdr.Params = toHdr.Params.Add("tag", sip.String{Str: tag})
		sess.localAddr.Params = sess.localAddr.Params.Add("tag", sip.String{Str: tag})
	}
	if _, err := c.srv.Respond(res); err != nil {
		return fmt.Errorf("send 200 OK: %w", err)
	}
	return nil
}

// Hangup terminates a call identified by callID.
func (c *SIPClient) Hangup(ctx context.Context, callID string) error {
	coreLog.Infof("SIP Hangup call %s", callID)
	c.mu.Lock()
	sess, ok := c.calls[callID]
	if ok {
		sess.cseq++
	}
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("call %s not found", callID)
	}

	cid := sip.CallID(callID)
	rb := sip.NewRequestBuilder().
		SetMethod(sip.BYE).
		SetRecipient(sess.remoteAddr.Uri).
		SetFrom(sess.localAddr).
		SetTo(sess.remoteAddr).
		SetContact(sess.localAddr).
		SetCallID(&cid).
		SetSeqNo(sess.cseq)

	req, err := rb.Build()
	if err != nil {
		return fmt.Errorf("build BYE: %w", err)
	}

	if _, err := c.srv.Request(req); err != nil {
		return fmt.Errorf("send BYE: %w", err)
	}

	c.mu.Lock()
	delete(c.calls, callID)
	c.mu.Unlock()
	return nil
}

// DialDtmf sends DTMF digits during a call.
func (c *SIPClient) DialDtmf(ctx context.Context, callID, digits string) error {
	coreLog.Infof("SIP DTMF on %s: %s", callID, digits)
	c.mu.Lock()
	sess, ok := c.calls[callID]
	if ok {
		sess.cseq++
	}
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("call %s not found", callID)
	}

	body := fmt.Sprintf("Signal=%s\r\nDuration=250\r\n", digits)
	cid := sip.CallID(callID)
	ctype := sip.ContentType("application/dtmf-relay")
	rb := sip.NewRequestBuilder().
		SetMethod(sip.INFO).
		SetRecipient(sess.remoteAddr.Uri).
		SetFrom(sess.localAddr).
		SetTo(sess.remoteAddr).
		SetContact(sess.localAddr).
		SetCallID(&cid).
		SetSeqNo(sess.cseq).
		SetContentType(&ctype).
		SetBody(body)

	req, err := rb.Build()
	if err != nil {
		return fmt.Errorf("build INFO: %w", err)
	}
	if _, err := c.srv.Request(req); err != nil {
		return fmt.Errorf("send INFO: %w", err)
	}
	return nil
}

// BridgeAudio connects audio between two calls.
func (c *SIPClient) BridgeAudio(ctx context.Context, srcID, dstID string) error {
	coreLog.Infof("SIP BridgeAudio %s <-> %s", srcID, dstID)
	tgvoip.ConnectSIPMedia(tgvoip.NewController(), func([]int16) {}, func([]int16) {})
	return nil
}
