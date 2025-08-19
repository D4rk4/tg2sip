package main

import (
	"fmt"
	"time"

	ini "gopkg.in/ini.v1"
)

// Settings holds application configuration loaded from settings.ini.
type Settings struct {
	sipPort        int
	sipPortRange   int
	publicAddress  string
	stunServer     string
	idURI          string
	callbackURI    string
	rawPCM         bool
	sipThreadCount int

	apiID              int
	apiHash            string
	dbFolder           string
	systemLanguageCode string
	deviceModel        string
	systemVersion      string
	applicationVersion string
	phoneNumber        string

	udpP2P       bool
	udpReflector bool
	aecEnabled   bool
	nsEnabled    bool
	agcEnabled   bool

	proxyEnabled  bool
	proxyAddress  string
	proxyPort     int
	proxyUsername string
	proxyPassword string

	voipProxyEnabled  bool
	voipProxyAddress  string
	voipProxyPort     int
	voipProxyUsername string
	voipProxyPassword string

	extraWaitTime int
	peerFloodTime int
}

// LoadSettings reads configuration from ini file and validates required fields.
func LoadSettings(cfg *ini.File) (*Settings, error) {
	s := &Settings{}

	sec := cfg.Section("sip")
	s.sipPort = sec.Key("port").MustInt(5060)
	s.sipPortRange = sec.Key("port_range").MustInt(0)
	s.publicAddress = sec.Key("public_address").String()
	s.stunServer = sec.Key("stun_server").String()
	s.idURI = sec.Key("id_uri").MustString("sip:localhost")
	s.callbackURI = sec.Key("callback_uri").String()
	s.rawPCM = sec.Key("raw_pcm").MustBool(true)
	s.sipThreadCount = sec.Key("thread_count").MustInt(1)

	sec = cfg.Section("telegram")
	s.apiID = sec.Key("api_id").MustInt(0)
	s.apiHash = sec.Key("api_hash").String()
	s.dbFolder = sec.Key("database_folder").MustString("/data")
	s.systemLanguageCode = sec.Key("system_language_code").MustString("en-US")
	s.deviceModel = sec.Key("device_model").MustString("PC")
	s.systemVersion = sec.Key("system_version").MustString("Linux")
	s.applicationVersion = sec.Key("application_version").MustString("1.0")
	s.phoneNumber = sec.Key("phone_number").String()

	s.udpP2P = sec.Key("udp_p2p").MustBool(false)
	s.udpReflector = sec.Key("udp_reflector").MustBool(true)
	s.aecEnabled = sec.Key("enable_aec").MustBool(false)
	s.nsEnabled = sec.Key("enable_ns").MustBool(false)
	s.agcEnabled = sec.Key("enable_agc").MustBool(false)

	s.proxyEnabled = sec.Key("use_proxy").MustBool(false)
	s.proxyAddress = sec.Key("proxy_address").String()
	s.proxyPort = sec.Key("proxy_port").MustInt(0)
	s.proxyUsername = sec.Key("proxy_username").String()
	s.proxyPassword = sec.Key("proxy_password").String()

	s.voipProxyEnabled = sec.Key("use_voip_proxy").MustBool(false)
	s.voipProxyAddress = sec.Key("voip_proxy_address").String()
	s.voipProxyPort = sec.Key("voip_proxy_port").MustInt(0)
	s.voipProxyUsername = sec.Key("voip_proxy_username").String()
	s.voipProxyPassword = sec.Key("voip_proxy_password").String()

	sec = cfg.Section("other")
	s.extraWaitTime = sec.Key("extra_wait_time").MustInt(30)
	s.peerFloodTime = sec.Key("peer_flood_time").MustInt(86400)

	if s.apiID == 0 || s.apiHash == "" {
		return nil, fmt.Errorf("telegram api settings must be set")
	}

	return s, nil
}

func (s *Settings) SIPPort() int          { return s.sipPort }
func (s *Settings) SIPPortRange() int     { return s.sipPortRange }
func (s *Settings) PublicAddress() string { return s.publicAddress }
func (s *Settings) StunServer() string    { return s.stunServer }
func (s *Settings) IDURI() string         { return s.idURI }
func (s *Settings) CallbackURI() string   { return s.callbackURI }
func (s *Settings) RawPCM() bool          { return s.rawPCM }
func (s *Settings) SIPThreadCount() int   { return s.sipThreadCount }

func (s *Settings) APIID() int                 { return s.apiID }
func (s *Settings) APIHash() string            { return s.apiHash }
func (s *Settings) DatabaseFolder() string     { return s.dbFolder }
func (s *Settings) SystemLanguageCode() string { return s.systemLanguageCode }
func (s *Settings) DeviceModel() string        { return s.deviceModel }
func (s *Settings) SystemVersion() string      { return s.systemVersion }
func (s *Settings) ApplicationVersion() string { return s.applicationVersion }
func (s *Settings) PhoneNumber() string        { return s.phoneNumber }

func (s *Settings) UDPP2P() bool       { return s.udpP2P }
func (s *Settings) UDPReflector() bool { return s.udpReflector }
func (s *Settings) AECEnabled() bool   { return s.aecEnabled }
func (s *Settings) NSEnabled() bool    { return s.nsEnabled }
func (s *Settings) AGCEnabled() bool   { return s.agcEnabled }

func (s *Settings) ProxyEnabled() bool    { return s.proxyEnabled }
func (s *Settings) ProxyAddress() string  { return s.proxyAddress }
func (s *Settings) ProxyPort() int        { return s.proxyPort }
func (s *Settings) ProxyUsername() string { return s.proxyUsername }
func (s *Settings) ProxyPassword() string { return s.proxyPassword }

func (s *Settings) VoipProxyEnabled() bool    { return s.voipProxyEnabled }
func (s *Settings) VoipProxyAddress() string  { return s.voipProxyAddress }
func (s *Settings) VoipProxyPort() int        { return s.voipProxyPort }
func (s *Settings) VoipProxyUsername() string { return s.voipProxyUsername }
func (s *Settings) VoipProxyPassword() string { return s.voipProxyPassword }

func (s *Settings) ExtraWaitTime() time.Duration {
	return time.Duration(s.extraWaitTime) * time.Second
}

func (s *Settings) PeerFloodTime() time.Duration {
	return time.Duration(s.peerFloodTime) * time.Second
}
