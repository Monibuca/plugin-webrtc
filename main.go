package webrtc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	. "github.com/Monibuca/engine/v2"
	"github.com/Monibuca/engine/v2/avformat"
	. "github.com/Monibuca/plugin-rtp"
	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v2"
	"github.com/pion/webrtc/v2/pkg/media"
)

var config struct {
	ICEServers []string
	PublicIP   string
}

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

var playWaitList WaitList
var reg_level = regexp.MustCompile("profile-level-id=(4.+f)")

type WaitList struct {
	m map[string]*WebRTC
	l sync.Mutex
}

func (wl *WaitList) Set(k string, v *WebRTC) {
	wl.l.Lock()
	defer wl.l.Unlock()
	if wl.m == nil {
		wl.m = make(map[string]*WebRTC)
	}
	wl.m[k] = v
}
func (wl *WaitList) Get(k string) *WebRTC {
	wl.l.Lock()
	defer wl.l.Unlock()
	defer delete(wl.m, k)
	return wl.m[k]
}
func init() {
	InstallPlugin(&PluginConfig{
		Config: &config,
		Name:   "WebRTC",
		Type:   PLUGIN_PUBLISHER | PLUGIN_SUBSCRIBER,
		Run:    run,
	})
}

type WebRTC struct {
	RTP
	*PeerConnection
	RemoteAddr string
	audioTrack *Track
	videoTrack *Track
	m          MediaEngine
	s          SettingEngine
	api        *API
	payloader  avformat.H264
	// codecs.H264Packet
	// *os.File
}

func (rtc *WebRTC) Play(streamPath string) bool {
	var sub Subscriber
	sub.ID = rtc.RemoteAddr
	sub.Type = "WebRTC"
	var lastTimeStampV, lastTiimeStampA uint32
	sub.OnData = func(packet *avformat.SendPacket) error {
		if packet.Type == avformat.FLV_TAG_TYPE_AUDIO {
			var s uint32
			if lastTiimeStampA > 0 {
				s = packet.Timestamp - lastTiimeStampA
			}
			lastTiimeStampA = packet.Timestamp
			rtc.audioTrack.WriteSample(media.Sample{
				Data:    packet.Payload[1:],
				Samples: s * 8,
			})
			return nil
		}
		if packet.IsSequence {
			rtc.payloader.PPS = sub.PPS
			rtc.payloader.SPS = sub.SPS
		} else {
			var s uint32
			if lastTimeStampV > 0 {
				s = packet.Timestamp - lastTimeStampV
			}
			lastTimeStampV = packet.Timestamp
			rtc.videoTrack.WriteSample(media.Sample{
				Data:    packet.Payload,
				Samples: s * 90,
			})
			// if packet.IsKeyFrame {
			// 	rtc.videoTrack.WriteSample(media.Sample{
			// 		Data:    sub.SPS,
			// 		Samples: 0,
			// 	})
			// 	rtc.videoTrack.WriteSample(media.Sample{
			// 		Data:    sub.PPS,
			// 		Samples: 0,
			// 	})
			// }
			// for payload := packet.Payload[5:]; len(payload) > 4; {
			// 	var naulLen = int(util.BigEndian.Uint32(payload))
			// 	payload = payload[4:]
			// 	rtc.videoTrack.WriteSample(media.Sample{
			// 		Data:    payload[:naulLen],
			// 		Samples: s * 90,
			// 	})
			// 	s = 0
			// 	payload = payload[naulLen:]
			// }
		}
		return nil
	}
	// go sub.Subscribe(streamPath)
	rtc.OnICEConnectionStateChange(func(connectionState ICEConnectionState) {
		Printf("%s Connection State has changed %s ", streamPath, connectionState.String())
		switch connectionState {
		case ICEConnectionStateDisconnected:
			sub.Close()
			rtc.Close()
		case ICEConnectionStateConnected:

			//rtc.videoTrack = rtc.GetSenders()[0].Track()
			sub.Subscribe(streamPath)
		}
	})
	return true
}
func (rtc *WebRTC) Publish(streamPath string) bool {
	rtc.m.RegisterCodec(NewRTPCodec(RTPCodecTypeVideo,
		H264,
		90000,
		0,
		"level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
		DefaultPayloadTypeH264,
		new(avformat.H264)))
	//m.RegisterCodec(NewRTPPCMUCodec(DefaultPayloadTypePCMU, 8000))
	rtc.s.SetNAT1To1IPs([]string{config.PublicIP}, ICECandidateTypeHost)
	rtc.api = NewAPI(WithMediaEngine(rtc.m), WithSettingEngine(rtc.s))
	peerConnection, err := rtc.api.NewPeerConnection(Configuration{
		ICEServers: []ICEServer{
			{
				URLs: config.ICEServers,
			},
		},
	})
	if _, err = peerConnection.AddTransceiverFromKind(RTPCodecTypeVideo); err != nil {
		if err != nil {
			Println(err)
			return false
		}
	}
	if err != nil {
		return false
	}
	peerConnection.OnICEConnectionStateChange(func(connectionState ICEConnectionState) {
		Printf("%s Connection State has changed %s ", streamPath, connectionState.String())
		switch connectionState {
		case ICEConnectionStateDisconnected, ICEConnectionStateFailed:
			if rtc.Stream != nil {
				rtc.Stream.Close()
			}
		}
	})
	rtc.PeerConnection = peerConnection
	if rtc.RTP.Publish(streamPath) {
		//f, _ := os.OpenFile("resource/live/rtc.h264", os.O_TRUNC|os.O_WRONLY, 0666)
		rtc.Stream.Type = "WebRTC"
		peerConnection.OnTrack(func(track *Track, receiver *RTPReceiver) {
			defer rtc.Stream.Close()
			go func() {
				ticker := time.NewTicker(time.Second * 2)
				select {
				case <-ticker.C:
					if rtcpErr := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: track.SSRC()}}); rtcpErr != nil {
						fmt.Println(rtcpErr)
					}
				case <-rtc.Done():
					return
				}
			}()
			pack := &RTPPack{
				Type: RTPType(track.Kind() - 1),
			}
			for b := make([]byte, 1460); ; rtc.PushPack(pack) {
				i, err := track.Read(b)
				if err != nil {
					return
				}
				if err = pack.Unmarshal(b[:i]); err != nil {
					return
				}
				// rtc.Unmarshal(pack.Payload)
				// f.Write(bytes)
			}
		})
	} else {
		return false
	}
	return true
}
func (rtc *WebRTC) GetAnswer() ([]byte, error) {
	// Sets the LocalDescription, and starts our UDP listeners
	answer, err := rtc.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}
	if err := rtc.SetLocalDescription(answer); err != nil {
		Println(err)
		return nil, err
	}
	if bytes, err := json.Marshal(answer); err != nil {
		Println(err)
		return bytes, err
	} else {
		return bytes, nil
	}
}

func run() {
	http.HandleFunc("/webrtc/play", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		origin := r.Header["Origin"]
		if len(origin) == 0 {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin[0])
		}

		w.Header().Set("Content-Type", "application/json")
		streamPath := r.URL.Query().Get("streamPath")
		var offer SessionDescription
		var rtc WebRTC
		bytes, err := ioutil.ReadAll(r.Body)
		defer func() {
			if err != nil {
				Println(err)
				fmt.Fprintf(w, `{"errmsg":"%s"}`, err)
				return
			}
			rtc.Play(streamPath)
		}()
		if err != nil {
			return
		}
		if err = json.Unmarshal(bytes, &offer); err != nil {
			return
		}

		pli := "42001f"
		if stream := FindStream(streamPath); stream != nil {
			pli = fmt.Sprintf("%x", stream.SPS[1:4])
		}
		if !strings.Contains(offer.SDP, pli) {
			pli = reg_level.FindAllStringSubmatch(offer.SDP, -1)[0][1]
		}
		rtc.m.RegisterCodec(NewRTPCodec(RTPCodecTypeVideo,
			H264,
			90000,
			0,
			"level-asymmetry-allowed=1;packetization-mode=1;profile-level-id="+pli,
			DefaultPayloadTypeH264,
			&rtc.payloader))
		rtc.m.RegisterCodec(NewRTPPCMACodec(DefaultPayloadTypePCMA, 8000))
		rtc.s.SetNAT1To1IPs([]string{config.PublicIP}, ICECandidateTypeHost)
		rtc.api = NewAPI(WithMediaEngine(rtc.m), WithSettingEngine(rtc.s))
		peerConnection, err := rtc.api.NewPeerConnection(Configuration{
			// ICEServers: []ICEServer{
			// 	{
			// 		URLs: config.ICEServers,
			// 	},
			// },
		})
		rtc.PeerConnection = peerConnection
		rtc.OnICECandidate(func(ice *ICECandidate) {
			if ice != nil {
				Println(ice.ToJSON().Candidate)
			}
		})
		// if r, err := peerConnection.AddTransceiverFromKind(RTPCodecTypeVideo); err == nil {
		// 	rtc.videoTrack = r.Sender().Track()
		// } else {
		// 	Println(err)
		// }
		if err != nil {
			return
		}
		rtc.RemoteAddr = r.RemoteAddr
		if err = rtc.SetRemoteDescription(offer); err != nil {
			return
		}
		// rtc.m.PopulateFromSDP(offer)
		// var vpayloadType uint8 = 0

		// for _, videoCodec := range rtc.m.GetCodecsByKind(RTPCodecTypeVideo) {
		// 	if videoCodec.Name == H264 {
		// 		vpayloadType = videoCodec.PayloadType
		// 		videoCodec.Payloader = &rtc.payloader
		// 		Printf("H264 fmtp %v", videoCodec.SDPFmtpLine)

		// 	}
		// }
		// println(vpayloadType)
		if rtc.videoTrack, err = rtc.NewTrack(DefaultPayloadTypeH264, 8, "video", "monibuca"); err != nil {
			return
		}
		if rtc.audioTrack, err = rtc.NewTrack(DefaultPayloadTypePCMA, 9, "audio", "monibuca"); err != nil {
			return
		}
		if _, err = rtc.AddTrack(rtc.videoTrack); err != nil {
			return
		}
		if bytes, err := rtc.GetAnswer(); err == nil {
			w.Write(bytes)
		} else {
			return
		}
	})

	http.HandleFunc("/webrtc/publish", func(w http.ResponseWriter, r *http.Request) {
		streamPath := r.URL.Query().Get("streamPath")
		offer := SessionDescription{}
		bytes, err := ioutil.ReadAll(r.Body)
		err = json.Unmarshal(bytes, &offer)
		if err != nil {
			Println(err)
			return
		}
		rtc := new(WebRTC)
		rtc.RemoteAddr = r.RemoteAddr
		if rtc.Publish(streamPath) {
			if err := rtc.SetRemoteDescription(offer); err != nil {
				Println(err)
				return
			}
			if bytes, err = rtc.GetAnswer(); err == nil {
				w.Write(bytes)
			} else {
				Println(err)
				w.Write([]byte(err.Error()))
				return
			}
		} else {
			w.Write([]byte("bad name"))
		}
	})
}
