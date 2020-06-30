package webrtc

import (
	"bytes"
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
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	. "github.com/pion/webrtc/v2"
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
var playWaitList sync.Map

func init() {
	m.RegisterCodec(NewRTPH264Codec(DefaultPayloadTypeH264, 90000))
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
			var sequence uint16
			var sub Subscriber
			var sps []byte
			var pps []byte
			sub.ID = rtc.RemoteAddr
			sub.Type = "WebRTC"
			nextHeader := func(ts uint32, marker bool) rtp.Header {
				sequence++
				return rtp.Header{
					Version:        2,
					SSRC:           SSRC,
					PayloadType:    DefaultPayloadTypeH264,
					SequenceNumber: sequence,
					Timestamp:      ts,
					Marker:         marker,
				}
			}
			stapA := func(naul ...[]byte) []byte {
				var buffer bytes.Buffer
				buffer.WriteByte(24)
				for _, n := range naul {
					l := len(n)
					buffer.WriteByte(byte(l >> 8))
					buffer.WriteByte(byte(l))
					buffer.Write(n)
				}
				return buffer.Bytes()
			}

			// aud := []byte{0x09, 0x30}
			sub.OnData = func(packet *avformat.SendPacket) error {
				if packet.Type == avformat.FLV_TAG_TYPE_AUDIO {
					return nil
				}
				if packet.IsSequence {
					payload := packet.Payload[11:]
					spsLen := int(payload[0])<<8 + int(payload[1])
					sps = payload[2:spsLen]
					payload = payload[3+spsLen:]
					ppsLen := int(payload[0])<<8 + int(payload[1])
					pps = payload[2:ppsLen]
				} else {
					if packet.IsKeyFrame {
						if err := rtc.videoTrack.WriteRTP(&rtp.Packet{
							Header:  nextHeader(packet.Timestamp*90, true),
							Payload: stapA(sps, pps),
						}); err != nil {
							return err
						}
					}
					payload := packet.Payload[5:]
					for {
						var naulLen = int(util.BigEndian.Uint32(payload))
						payload = payload[4:]
						_payload := payload[:naulLen]
						if naulLen > 1000 {
							indicator := (_payload[0] & 224) | 28
							nalutype := _payload[0] & 31
							header := 128 | nalutype
							part := _payload[1:1000]
							marker := false
							for {
								if err := rtc.videoTrack.WriteRTP(&rtp.Packet{
									Header:  nextHeader(packet.Timestamp*90, marker),
									Payload: append([]byte{indicator, header}, part...),
								}); err != nil {
									return err
								}
								if _payload == nil {
									break
								}
								_payload = _payload[1000:]
								if len(_payload) <= 1000 {
									header = 64 | nalutype
									part = _payload
									_payload = nil
									marker = true
								} else {
									header = nalutype
									part = _payload[:1000]
								}
							}
						} else {
							if err := rtc.videoTrack.WriteRTP(&rtp.Packet{
								Header:  nextHeader(packet.Timestamp*90, true),
								Payload: _payload,
							}); err != nil {
								return err
							}
						}
						if len(payload) < naulLen+4 {
							break
						}
						payload = payload[naulLen:]
					}
					// if err := videoTrack.WriteRTP(&rtp.Packet{
					// 	Header:  nextHeader(packet.Timestamp * 90),
					// 	Payload: aud,
					// }); err != nil {
					// 	return err
					// }
				}
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
		var h264 codecs.H264Packet
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
				h264.Unmarshal(pack.Payload)
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
		if value, ok := playWaitList.Load(streamPath); ok {
			rtc := value.(*WebRTC)
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
		playWaitList.Store(streamPath, rtc)
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
			w.Write([]byte(`success`))
		} else {
			w.Write([]byte(`{"errmsg":"bad name"}`))
		}
	})
}
