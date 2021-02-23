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

	"github.com/Monibuca/engine/v3"
	"github.com/Monibuca/utils/v3"
	"github.com/Monibuca/utils/v3/codec"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	. "github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

var config struct {
	ICEServers []string
	PublicIP   []string
	PortMin    uint16
	PortMax    uint16
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
var (
	playWaitList WaitList
	reg_level    = regexp.MustCompile("profile-level-id=(4.+f)")
	api          *API
)

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
	engine.InstallPlugin(&engine.PluginConfig{
		Config: &config,
		Name:   "WebRTC",
		Run:    run,
	})
}

type WebRTC struct {
	engine.Publisher
	*PeerConnection
}

func (rtc *WebRTC) Publish(streamPath string) bool {
	// rtc.m.RegisterCodec(NewRTPCodec(RTPCodecTypeVideo,
	// 	H264,
	// 	90000,
	// 	0,
	// 	"level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
	// 	DefaultPayloadTypeH264,
	// 	new(codec.H264)))

	// rtc.m.RegisterCodec(RTPCodecParameters{
	// 	RTPCodecCapability: RTPCodecCapability{MimeType: "video/h264", ClockRate: 90000, Channels: 0, SDPFmtpLine: "", RTCPFeedback: nil},
	// 	PayloadType:        96,
	// 	}, RTPCodecTypeVideo);

	//m.RegisterCodec(NewRTPPCMUCodec(DefaultPayloadTypePCMU, 8000))
	// if !strings.HasPrefix(rtc.RemoteAddr, "127.0.0.1") && !strings.HasPrefix(rtc.RemoteAddr, "[::1]") {
	// 	rtc.s.SetNAT1To1IPs(config.PublicIP, ICECandidateTypeHost)
	// }

	peerConnection, err := api.NewPeerConnection(Configuration{
		// ICEServers: []ICEServer{
		// 	{
		// 		URLs: config.ICEServers,
		// 	},
		// },
	})
	if err != nil {
		utils.Println(err)
		return false
	}
	if _, err = peerConnection.AddTransceiverFromKind(RTPCodecTypeVideo); err != nil {
		if err != nil {
			utils.Println(err)
			return false
		}
	}
	if err != nil {
		return false
	}
	peerConnection.OnICEConnectionStateChange(func(connectionState ICEConnectionState) {
		utils.Printf("%s Connection State has changed %s ", streamPath, connectionState.String())
		switch connectionState {
		case ICEConnectionStateDisconnected, ICEConnectionStateFailed:
			if rtc.Stream != nil {
				rtc.Stream.Close()
			}
		}
	})
	rtc.PeerConnection = peerConnection
	if rtc.Publish(streamPath) {
		//f, _ := os.OpenFile("resource/live/rtc.h264", os.O_TRUNC|os.O_WRONLY, 0666)
		rtc.Stream.Type = "WebRTC"
		peerConnection.OnTrack(func(track *TrackRemote, receiver *RTPReceiver) {
			defer rtc.Stream.Close()
			go func() {
				ticker := time.NewTicker(time.Second * 2)
				select {
				case <-ticker.C:
					if rtcpErr := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}}); rtcpErr != nil {
						fmt.Println(rtcpErr)
					}
				case <-rtc.Done():
					return
				}
			}()
			var etrack engine.Track
			if track.Kind() == RTPCodecTypeAudio {
				//TODO: 判断音频格式
				at := engine.NewAudioTrack()
				at.SoundFormat = 7
				rtc.SetOriginAT(at)
				etrack = at
			} else {
				vt := engine.NewVideoTrack()
				vt.CodecID = 7
				rtc.SetOriginVT(vt)
				etrack = vt
			}
			var pack rtp.Packet
			b := make([]byte, 1460)
			for {
				i, _, err := track.Read(b)
				if err != nil {
					return
				}
				if err = pack.Unmarshal(b[:i]); err != nil {
					return
				}
				etrack.PushRTP(pack)
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
	gatherComplete := GatheringCompletePromise(rtc.PeerConnection)
	if err := rtc.SetLocalDescription(answer); err != nil {
		utils.Println(err)
		return nil, err
	}
	<-gatherComplete
	if bytes, err := json.Marshal(rtc.LocalDescription()); err != nil {
		utils.Println(err)
		return bytes, err
	} else {
		return bytes, nil
	}
}

func run() {
	var m MediaEngine
	var s SettingEngine
	if config.PortMin > 0 && config.PortMax > 0 {
		s.SetEphemeralUDPPortRange(config.PortMin, config.PortMax)
	}
	m.RegisterDefaultCodecs()
	api = NewAPI(WithMediaEngine(&m), WithSettingEngine(s))
	http.HandleFunc("/webrtc/play", func(w http.ResponseWriter, r *http.Request) {
		utils.CORS(w, r)
		w.Header().Set("Content-Type", "application/json")
		streamPath := r.URL.Query().Get("streamPath")
		var offer SessionDescription
		var rtc WebRTC
		sub := engine.Subscriber{
			ID:   r.RemoteAddr,
			Type: "WebRTC",
		}
		bytes, err := ioutil.ReadAll(r.Body)
		defer func() {
			if err != nil {
				utils.Println(err)
				fmt.Fprintf(w, `{"errmsg":"%s"}`, err)
				return
			}
		}()
		if err != nil {
			return
		}
		if err = json.Unmarshal(bytes, &offer); err != nil {
			return
		}

		if err = sub.Subscribe(streamPath); err != nil {
			return
		}

		if rtc.PeerConnection, err = api.NewPeerConnection(Configuration{}); err != nil {
			return
		}
		rtc.OnICECandidate(func(ice *ICECandidate) {
			if ice != nil {
				utils.Println(ice.ToJSON().Candidate)
			}
		})
		if err = rtc.SetRemoteDescription(offer); err != nil {
			return
		}
		vt := sub.GetVideoTrack("h264")
		if vt != nil {
			pli := "42001f"
			pli = fmt.Sprintf("%x", vt.SPS[1:4])
			if !strings.Contains(offer.SDP, pli) {
				pli = reg_level.FindAllStringSubmatch(offer.SDP, -1)[0][1]
			}
			var videoTrack *TrackLocalStaticSample
			if videoTrack, err = NewTrackLocalStaticSample(RTPCodecCapability{MimeType: MimeTypeH264, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + pli}, "video", "m7s"); err != nil {
				return
			}
			if _, err = rtc.AddTrack(videoTrack); err != nil {
				return
			}
			var lastTimeStampV uint32
			sub.OnVideo = func(pack engine.VideoPack) {
				var s uint32
				if lastTimeStampV > 0 {
					s = pack.Timestamp - lastTimeStampV
				}
				lastTimeStampV = pack.Timestamp
				if pack.NalType == codec.NALU_IDR_Picture {
					videoTrack.WriteSample(media.Sample{
						Data: vt.SPS,
					})
					videoTrack.WriteSample(media.Sample{
						Data: vt.PPS,
					})
				}
				videoTrack.WriteSample(media.Sample{
					Data:     pack.Payload,
					Duration: time.Millisecond * time.Duration(s),
				})
			}
		}
		at := sub.GetAudioTrack("pcma", "pcmu")
		if at != nil {
			var audioTrack *TrackLocalStaticSample
			audioMimeType := MimeTypePCMA
			if at.SoundFormat == 8 {
				audioMimeType = MimeTypePCMU
			}
			if audioTrack, err = NewTrackLocalStaticSample(RTPCodecCapability{audioMimeType, 8000, 0, "", nil}, "audio", "m7s"); err != nil {
				return
			}
			if _, err = rtc.AddTrack(audioTrack); err != nil {
				return
			}
			var lastTimeStampA uint32
			sub.OnAudio = func(pack engine.AudioPack) {
				var s uint32
				if lastTimeStampA > 0 {
					s = pack.Timestamp - lastTimeStampA
				}
				lastTimeStampA = pack.Timestamp
				audioTrack.WriteSample(media.Sample{
					Data: pack.Payload, Duration: time.Millisecond * time.Duration(s),
				})
			}
		}

		if bytes, err := rtc.GetAnswer(); err == nil {
			w.Write(bytes)
			rtc.OnICEConnectionStateChange(func(connectionState ICEConnectionState) {
				utils.Printf("%s Connection State has changed %s ", streamPath, connectionState.String())
				switch connectionState {
				case ICEConnectionStateDisconnected, ICEConnectionStateFailed:
					sub.Close()
					rtc.PeerConnection.Close()
				case ICEConnectionStateConnected:
					if at != nil {
						go sub.PlayAudio(at)
					}
					if vt != nil {
						go sub.PlayVideo(vt)
					}
				}
			})
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
			utils.Println(err)
			return
		}
		rtc := new(WebRTC)
		if rtc.Publish(streamPath) {
			if err := rtc.SetRemoteDescription(offer); err != nil {
				utils.Println(err)
				return
			}
			if bytes, err = rtc.GetAnswer(); err == nil {
				w.Write(bytes)
			} else {
				utils.Println(err)
				w.Write([]byte(err.Error()))
				return
			}
		} else {
			w.Write([]byte("bad name"))
		}
	})
}
