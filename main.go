package webrtc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	. "github.com/Monibuca/engine/v2"
	"github.com/Monibuca/engine/v2/avformat"
	"github.com/Monibuca/engine/v2/util"
	. "github.com/Monibuca/plugin-rtp"
	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v2"
	"github.com/pion/webrtc/v2/pkg/media"
)

var config struct {
	ICEServers []string
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

var m MediaEngine
var api *API
var SSRC uint32
var SSRCMap = make(map[string]uint32)
var ssrcLock sync.Mutex
var playWaitList WaitList

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
	m.RegisterCodec(NewRTPCodec(RTPCodecTypeVideo,
		H264,
		90000,
		0,
		"level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
		DefaultPayloadTypeH264,
		new(avformat.H264)))
	//m.RegisterCodec(NewRTPPCMUCodec(DefaultPayloadTypePCMU, 8000))
	api = NewAPI(WithMediaEngine(m))
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
	videoTrack *Track
	// codecs.H264Packet
	// *os.File
}

func (rtc *WebRTC) Play(streamPath string) bool {
	rtc.OnICEConnectionStateChange(func(connectionState ICEConnectionState) {
		Printf("%s Connection State has changed %s ", streamPath, connectionState.String())
		switch connectionState {
		case ICEConnectionStateDisconnected:
			if rtc.Stream != nil {
				rtc.Stream.Close()
			}
		case ICEConnectionStateConnected:
			var sub Subscriber
			sub.ID = rtc.RemoteAddr
			sub.Type = "WebRTC"
			var lastTimeStamp uint32
			sub.OnData = func(packet *avformat.SendPacket) error {
				if packet.Type == avformat.FLV_TAG_TYPE_AUDIO {
					return nil
				}
				if packet.IsSequence {
				} else {
					var s uint32
					if lastTimeStamp > 0 {
						s = packet.Timestamp - lastTimeStamp
					}
					if packet.IsKeyFrame {
						rtc.videoTrack.WriteSample(media.Sample{
							Data:    sub.SPS,
							Samples: 0,
						})
						rtc.videoTrack.WriteSample(media.Sample{
							Data:    sub.PPS,
							Samples: 0,
						})
					}
					for payload := packet.Payload[5:]; len(payload) > 4; {
						var naulLen = int(util.BigEndian.Uint32(payload))
						payload = payload[4:]
						rtc.videoTrack.WriteSample(media.Sample{
							Data:    payload[:naulLen],
							Samples: s * 90,
						})
						s = 0
						payload = payload[naulLen:]
					}
				}
				lastTimeStamp = packet.Timestamp
				return nil
			}
			go sub.Subscribe(streamPath)
		}
	})
	return true
}
func (rtc *WebRTC) Publish(streamPath string) bool {
	peerConnection, err := api.NewPeerConnection(Configuration{
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
func (rtc *WebRTC) GetAnswer(localSdp SessionDescription) ([]byte, error) {
	// Sets the LocalDescription, and starts our UDP listeners
	if err := rtc.SetLocalDescription(localSdp); err != nil {
		Println(err)
		return nil, err
	}
	if bytes, err := json.Marshal(localSdp); err != nil {
		Println(err)
		return bytes, err
	} else {
		return bytes, nil
	}
}

func run() {
	http.HandleFunc("/webrtc/play", func(w http.ResponseWriter, r *http.Request) {
		streamPath := r.URL.Query().Get("streamPath")
		offer := SessionDescription{}
		bytes, err := ioutil.ReadAll(r.Body)
		err = json.Unmarshal(bytes, &offer)
		if err != nil {
			Println(err)
			return
		}
		if rtc := playWaitList.Get(streamPath); rtc != nil {
			if err := rtc.SetRemoteDescription(offer); err != nil {
				Println(err)
				return
			}
			if rtc.Play(streamPath) {
				w.Write([]byte(`success`))
			} else {
				w.Write([]byte(`{"errmsg":"bad name"}`))
			}
		} else {
			w.Write([]byte(`{"errmsg":"bad name"}`))
		}
	})
	http.HandleFunc("/webrtc/preparePlay", func(w http.ResponseWriter, r *http.Request) {
		streamPath := r.URL.Query().Get("streamPath")
		rtc := new(WebRTC)
		peerConnection, err := api.NewPeerConnection(Configuration{
			ICEServers: []ICEServer{
				{
					URLs: config.ICEServers,
				},
			},
		})
		if _, err = peerConnection.AddTransceiverFromKind(RTPCodecTypeVideo); err != nil {
			if err != nil {
				Println(err)
				return
			}
		}
		if err != nil {
			return
		}

		rtc.PeerConnection = peerConnection
		// Create a video track, using the same SSRC as the incoming RTP Packet
		ssrcLock.Lock()
		if _, ok := SSRCMap[streamPath]; !ok {
			SSRC++
			SSRCMap[streamPath] = SSRC
		}
		ssrcLock.Unlock()
		videoTrack, err := rtc.NewTrack(DefaultPayloadTypeH264, SSRC, "video", "monibuca")
		if err != nil {
			Println(err)
			return
		}
		if _, err = rtc.AddTrack(videoTrack); err != nil {
			Println(err)
			return
		}
		rtc.videoTrack = videoTrack
		playWaitList.Set(streamPath, rtc)
		rtc.RemoteAddr = r.RemoteAddr
		offer, err := rtc.CreateOffer(nil)
		if err != nil {
			Println(err)
			return
		}
		if bytes, err := rtc.GetAnswer(offer); err == nil {
			w.Write(bytes)
		} else {
			Println(err)
			w.Write([]byte(err.Error()))
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
			answer, err := rtc.CreateAnswer(nil)
			if err != nil {
				Println(err)
				return
			}
			if bytes, err = rtc.GetAnswer(answer); err == nil {
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
