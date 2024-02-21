package webrtc

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"m7s.live/engine/v4"

	_ "embed"

	"github.com/pion/interceptor"
	. "github.com/pion/webrtc/v4"
	"m7s.live/engine/v4/config"
	"m7s.live/engine/v4/util"
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

var (
	//go:embed publish.html
	publishHTML []byte

	//go:embed subscribe.html
	subscribeHTML []byte
	webrtcConfig  WebRTCConfig
	reg_level     = regexp.MustCompile("profile-level-id=(4.+f)")
	WebRTCPlugin  = engine.InstallPlugin(&webrtcConfig)
)

type WebRTCConfig struct {
	config.Publish
	config.Subscribe
	ICEServers []ICEServer   `desc:"ice服务器配置"`
	PublicIP   string        `desc:"公网IP"`
	PublicIPv6 string        `desc:"公网IPv6"`
	Port       string        `default:"tcp:9000" desc:"监听端口"`
	PLI        time.Duration `default:"2s" desc:"发送PLI请求间隔"`    // 视频流丢包后，发送PLI请求
	EnableOpus bool          `default:"true" desc:"是否启用opus编码"` // 是否启用opus编码
	EnableAv1  bool          `default:"true" desc:"是否启用av1编码"`  // 是否启用av1编码
	m          MediaEngine
	s          SettingEngine
	api        *API
}

func (conf *WebRTCConfig) OnEvent(event any) {
	switch event.(type) {
	case engine.FirstConfig:
		if len(conf.ICEServers) > 0 {
			for i := range conf.ICEServers {
				b, _ := conf.ICEServers[i].MarshalJSON()
				conf.ICEServers[i].UnmarshalJSON(b)
			}
		}
		webrtc.RegisterCodecs(&conf.m)
		if conf.EnableOpus {
			conf.m.RegisterCodec(RTPCodecParameters{
				RTPCodecCapability: RTPCodecCapability{MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", nil},
				PayloadType:        111,
			}, RTPCodecTypeAudio)
		}
		if conf.EnableAv1 {
			conf.m.RegisterCodec(RTPCodecParameters{
				RTPCodecCapability: RTPCodecCapability{MimeTypeAV1, 90000, 0, "profile=2;level-idx=8;tier=1", nil},
				PayloadType:        45,
			}, RTPCodecTypeVideo)
		}
		i := &interceptor.Registry{}
		if conf.PublicIP != "" {
			ips := []string{conf.PublicIP}
			if conf.PublicIPv6 != "" {
				ips = append(ips, conf.PublicIPv6)
			}
			conf.s.SetNAT1To1IPs(ips, ICECandidateTypeHost)
		}
		protocol, ports := util.Conf2Listener(conf.Port)
		if len(ports) == 0 {
			WebRTCPlugin.Fatal("webrtc port config error")
		}
		if protocol == "tcp" {
			tcpport := int(ports[0])
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
		} else if len(ports) == 2 {
			conf.s.SetEphemeralUDPPortRange(ports[0], ports[1])
		} else {
			// 创建共享WEBRTC端口 默认9000
			udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
				IP:   net.IP{0, 0, 0, 0},
				Port: int(ports[0]),
			})
			if err != nil {
				WebRTCPlugin.Fatal("webrtc listener udp", zap.Error(err))
			}
			WebRTCPlugin.Info("webrtc start listen", zap.Uint16("port", ports[0]))
			conf.s.SetICEUDPMux(NewICEUDPMux(nil, udpListener))
			conf.s.SetNetworkTypes([]NetworkType{NetworkTypeUDP4, NetworkTypeUDP6})
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
	rawQuery := r.URL.RawQuery
	bytes, err := io.ReadAll(r.Body)
	var suber WebRTCSubscriber
	suber.SDP = string(bytes)
	suber.RemoteAddr = r.RemoteAddr
	if suber.PeerConnection, err = conf.api.NewPeerConnection(Configuration{
		ICEServers: conf.ICEServers,
	}); err != nil {
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
	if rawQuery != "" {
		streamPath += "?" + rawQuery
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

// https://datatracker.ietf.org/doc/html/draft-ietf-wish-whip
func (conf *WebRTCConfig) Push_(w http.ResponseWriter, r *http.Request) {
	streamPath := r.URL.Path[len("/push/"):]
	rawQuery := r.URL.RawQuery
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		auth = auth[len("Bearer "):]
		if rawQuery != "" {
			rawQuery += "&bearer=" + auth
		} else {
			rawQuery = "bearer=" + auth
		}
		WebRTCPlugin.Info("push", zap.String("stream", streamPath), zap.String("bearer", auth))
	}
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/webrtc/api/stop/push/"+streamPath)
	if rawQuery != "" {
		streamPath += "?" + rawQuery
	}
	bytes, err := io.ReadAll(r.Body)
	var puber WebRTCPublisher
	puber.SDP = string(bytes)
	if puber.PeerConnection, err = conf.api.NewPeerConnection(Configuration{
		ICEServers: conf.ICEServers,
	}); err != nil {
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
		puber.Info("Connection State has changed:" + state.String())
		switch state {
		case PeerConnectionStateConnected:

		case PeerConnectionStateDisconnected, PeerConnectionStateFailed, PeerConnectionStateClosed:
			puber.Stop()
		}
	})
	if err := puber.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: puber.SDP}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if answer, err := puber.GetAnswer(); err == nil {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, answer)
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func (conf *WebRTCConfig) Test_Publish(w http.ResponseWriter, r *http.Request) {
	w.Write(publishHTML)
}
func (conf *WebRTCConfig) Test_ScreenShare(w http.ResponseWriter, r *http.Request) {
	w.Write(publishHTML)
}
func (conf *WebRTCConfig) Test_Subscribe(w http.ResponseWriter, r *http.Request) {
	w.Write(subscribeHTML)
}

func (conf *WebRTCConfig) Batch(w http.ResponseWriter, r *http.Request) {
	bytes, err := io.ReadAll(r.Body)
	var suber WebRTCBatcher
	suber.RemoteAddr = r.RemoteAddr
	suber.SDP = string(bytes)
	if suber.PeerConnection, err = conf.api.NewPeerConnection(Configuration{
		ICEServers: conf.ICEServers,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = suber.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sdp, err := suber.GetAnswer(); err == nil {
		w.Header().Set("Content-Type", "application/sdp")
		fmt.Fprintf(w, "%s", sdp)
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
