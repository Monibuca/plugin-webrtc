package webrtc

import (
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"m7s.live/engine/v4"

	_ "embed"

	"github.com/pion/interceptor"
	. "github.com/pion/webrtc/v3"
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

//	type udpConn struct {
//		conn *net.UDPConn
//		port int
//	}

//go:embed publish.html
var publishHTML []byte

//go:embed subscribe.html
var subscribeHTML []byte
var (
	reg_level = regexp.MustCompile("profile-level-id=(4.+f)")
)

type WebRTCConfig struct {
	config.Publish
	config.Subscribe
	ICEServers []string
	PublicIP   []string
	Port       string        `default:"tcp:9000"`
	PLI        time.Duration `default:"2s"` // 视频流丢包后，发送PLI请求
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

		protocol, port, _ := strings.Cut(conf.Port, ":")
		if protocol == "tcp" {
			tcpport, _ := strconv.Atoi(port)
			tcpl, err := net.ListenTCP("tcp", &net.TCPAddr{
				IP:   net.IP{0, 0, 0, 0},
				Port: tcpport,
			})
			if err != nil {
				WebRTCPlugin.Fatal("webrtc listener tcp", zap.Error(err))
			}
			WebRTCPlugin.Info("webrtc start listen", zap.Int("port", tcpport))
			conf.s.SetICETCPMux(NewICETCPMux(nil, tcpl, 4096))
			conf.s.SetNetworkTypes([]NetworkType{NetworkTypeTCP4, NetworkTypeTCP6})
		} else {
			r := strings.Split(port, "-")
			if len(r) == 2 {
				min, _ := strconv.Atoi(r[0])
				max, _ := strconv.Atoi(r[1])
				conf.s.SetEphemeralUDPPortRange(uint16(min), uint16(max))
			} else {
				udpport, _ := strconv.Atoi(port)
				// 创建共享WEBRTC端口 默认9000
				udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
					IP:   net.IP{0, 0, 0, 0},
					Port: udpport,
				})
				if err != nil {
					WebRTCPlugin.Fatal("webrtc listener udp", zap.Error(err))
				}
				WebRTCPlugin.Info("webrtc start listen", zap.Int("port", udpport))
				conf.s.SetICEUDPMux(NewICEUDPMux(nil, udpListener))
				conf.s.SetNetworkTypes([]NetworkType{NetworkTypeUDP4, NetworkTypeUDP6})
			}
		}

		if err := RegisterDefaultInterceptors(&conf.m, i); err != nil {
			panic(err)
		}
		conf.api = NewAPI(WithMediaEngine(&conf.m),
			WithInterceptorRegistry(i), WithSettingEngine(conf.s))
	}
}

func (conf *WebRTCConfig) Play_(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/sdp")
	streamPath := r.URL.Path[len("/play/"):]
	bytes, err := ioutil.ReadAll(r.Body)
	var suber WebRTCSubscriber
	suber.SDP = string(bytes)
	if suber.PeerConnection, err = conf.api.NewPeerConnection(Configuration{}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	suber.OnICECandidate(func(ice *ICECandidate) {
		if ice != nil {
			suber.Info(ice.ToJSON().Candidate)
		}
	})
	if err = suber.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: suber.SDP}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = WebRTCPlugin.Subscribe(streamPath, &suber); err != nil {
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
	streamPath := r.URL.Path[len("/push/"):]
	w.Header().Set("Content-Type", "application/sdp")
	bytes, err := ioutil.ReadAll(r.Body)
	var puber WebRTCPublisher
	puber.SDP = string(bytes)
	if puber.PeerConnection, err = conf.api.NewPeerConnection(Configuration{}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	puber.SetIO(puber.PeerConnection) //TODO: 单PC需要注释掉
	puber.OnICECandidate(func(ice *ICECandidate) {
		if ice != nil {
			puber.Info(ice.ToJSON().Candidate)
		}
	})
	puber.OnDataChannel(func(d *DataChannel) {
		puber.Info("OnDataChannel", zap.String("label", d.Label()))
		d.OnMessage(func(msg DataChannelMessage) {
			puber.SDP = string(msg.Data[1:])
			puber.Debug("dc message", zap.String("sdp", puber.SDP))
			if err := puber.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: puber.SDP}); err != nil {
				return
			}
			if answer, err := puber.GetAnswer(); err == nil {
				d.SendText(answer)
			} else {
				return
			}
			switch msg.Data[0] {
			case '0':
				puber.Stop()
			case '1':

			}
		})
	})
	// if _, err = puber.AddTransceiverFromKind(RTPCodecTypeVideo); err != nil {
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// 	return
	// }
	// if _, err = puber.AddTransceiverFromKind(RTPCodecTypeAudio); err != nil {
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// 	return
	// }
	if err = WebRTCPlugin.Publish(streamPath, &puber); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	puber.OnConnectionStateChange(func(state PeerConnectionState) {
		switch state {
		case PeerConnectionStateConnected:

		case PeerConnectionStateDisconnected, PeerConnectionStateFailed:
			puber.Stop()
		}
	})
	if err := puber.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: puber.SDP}); err != nil {
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

func (conf *WebRTCConfig) Test_Publish(w http.ResponseWriter, r *http.Request) {
	w.Write(publishHTML)
}
func (conf *WebRTCConfig) Test_Subscribe(w http.ResponseWriter, r *http.Request) {
	w.Write(subscribeHTML)
}

var webrtcConfig WebRTCConfig

var WebRTCPlugin = engine.InstallPlugin(&webrtcConfig)

func (conf *WebRTCConfig) Batch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/sdp")
	bytes, err := ioutil.ReadAll(r.Body)
	var suber WebRTCBatcher
	suber.SDP = string(bytes)
	if suber.PeerConnection, err = conf.api.NewPeerConnection(Configuration{}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = suber.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sdp, err := suber.GetAnswer(); err == nil {
		w.Write([]byte(sdp))
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
