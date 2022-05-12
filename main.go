package webrtc

import (
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"github.com/pion/interceptor"

	. "github.com/pion/webrtc/v3"
	"m7s.live/engine/v4"
	"m7s.live/engine/v4/config"
	"m7s.live/plugin/webrtc/v4/webrtc"
)

// }{[]string{
// 	"stun:stun.ekiga.net",
// 	"stun:stun.ideasip.com",
// 	"stun:stun.schlund.de",
// 	"stun:stun.stunprotocol.org:3478",
// 	"stun:stun.voiparound.com",
// 	"stun:stun.voipbuster.com",
// 	"stun:stun.voipstunt.com",
// 	"stun:stun.voxgratia.org",
// 	"stun:stun.services.mozilla.com",
// 	"stun:stun.xten.com",
// 	"stun:stun.softjoys.com",
// 	"stun:stunserver.org",
// 	"stun:stun.schlund.de",
// 	"stun:stun.rixtelecom.se",
// 	"stun:stun.iptel.org",
// 	"stun:stun.ideasip.com",
// 	"stun:stun.fwdnet.net",
// 	"stun:stun.ekiga.net",
// 	"stun:stun01.sipphone.com",
// }}

// type udpConn struct {
// 	conn *net.UDPConn
// 	port int
// }
var (
	reg_level = regexp.MustCompile("profile-level-id=(4.+f)")
	api       *API
)

type WebRTCConfig struct {
	config.Publish
	config.Subscribe
	ICEServers []string
	PublicIP   []string
	PortMin    uint16
	PortMax    uint16
	PLI        time.Duration
	m          MediaEngine
	s          SettingEngine
	api        *API
}

func (conf *WebRTCConfig) OnEvent(event any) {
	switch event.(type) {
	case engine.FirstConfig:
		webrtc.RegisterCodecs(&conf.m)
		i := &interceptor.Registry{}
		if len(conf.PublicIP) > 0 {
			conf.s.SetNAT1To1IPs(conf.PublicIP, ICECandidateTypeHost)
		}
		if conf.PortMin > 0 && conf.PortMax > 0 {
			conf.s.SetEphemeralUDPPortRange(conf.PortMin, conf.PortMax)
		}
		if len(conf.PublicIP) > 0 {
			conf.s.SetNAT1To1IPs(conf.PublicIP, ICECandidateTypeHost)
		}
		conf.s.SetNetworkTypes([]NetworkType{NetworkTypeUDP4, NetworkTypeUDP6})
		if err := RegisterDefaultInterceptors(&conf.m, i); err != nil {
			panic(err)
		}
		conf.api = NewAPI(WithMediaEngine(&conf.m),
			WithInterceptorRegistry(i), WithSettingEngine(conf.s))
	}
}

func (conf *WebRTCConfig) Play_(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/sdp")
	streamPath := r.URL.Path[len("/webrtc/play/"):]
	bytes, err := ioutil.ReadAll(r.Body)
	var suber WebRTCSubscriber
	suber.SDP = string(bytes)
	if suber.PeerConnection, err = api.NewPeerConnection(Configuration{}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	suber.OnICECandidate(func(ice *ICECandidate) {
		if ice != nil {
			suber.Info(ice.ToJSON().Candidate)
		}
	})
	if err = suber.SetRemoteDescription(SessionDescription{SDP: suber.SDP}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = plugin.Subscribe(streamPath, &suber); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if sdp, err := suber.GetAnswer(); err == nil {
		w.Write([]byte(sdp))
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func (conf *WebRTCConfig) Push_(w http.ResponseWriter, r *http.Request) {
	streamPath := r.URL.Path[len("/webrtc/push/"):]
	w.Header().Set("Content-Type", "application/sdp")
	bytes, err := ioutil.ReadAll(r.Body)
	var puber WebRTCPublisher
	puber.SDP = string(bytes)
	if puber.PeerConnection, err = api.NewPeerConnection(Configuration{}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	puber.OnICECandidate(func(ice *ICECandidate) {
		if ice != nil {
			puber.Info(ice.ToJSON().Candidate)
		}
	})
	if _, err = puber.AddTransceiverFromKind(RTPCodecTypeVideo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err = puber.AddTransceiverFromKind(RTPCodecTypeAudio); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = plugin.Publish(streamPath, &puber); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := puber.SetRemoteDescription(SessionDescription{SDP: puber.SDP}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if answer, err := puber.GetAnswer(); err == nil {
		w.Write([]byte(answer))
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

var webrtcConfig = &WebRTCConfig{
	PLI: time.Second * 2,
}

var plugin = engine.InstallPlugin(webrtcConfig)
