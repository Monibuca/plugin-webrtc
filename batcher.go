package webrtc

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v3"
	"go.uber.org/zap"
	. "m7s.live/engine/v4"
	. "m7s.live/engine/v4/track"
)

type Signal struct {
	Type       string   `json:"type"`
	StreamList []string `json:"streamList"`
	Offer      string   `json:"offer"`
	Answer     string   `json:"answer"`
	StreamPath string   `json:"streamPath"`
}
type BatchUplink struct {
	Publisher
	StreamPath string
}
type WebRTCBatcher struct {
	WebRTCIO
	PageSize      int
	PageNum       int
	subscribers   []*WebRTCBatchSubscriber
	signalChannel *DataChannel
	BatchUplink
}

func (suber *WebRTCBatcher) Start() (err error) {
	suber.OnICECandidate(func(ice *ICECandidate) {
		if ice != nil {
			WebRTCPlugin.Info(ice.ToJSON().Candidate)
		}
	})
	suber.OnDataChannel(func(d *DataChannel) {
		WebRTCPlugin.Info("OnDataChannel:" + d.Label())
		suber.signalChannel = d
		suber.signalChannel.OnMessage(suber.Signal)
	})
	if err = suber.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: suber.SDP}); err != nil {
		return
	}
	suber.OnConnectionStateChange(func(pcs PeerConnectionState) {
		WebRTCPlugin.Info("Connection State has changed:" + pcs.String())
		switch pcs {
		case PeerConnectionStateConnected:
			suber.OnTrack(func(track *TrackRemote, receiver *RTPReceiver) {
				if suber.Publisher.Stream == nil {
					WebRTCPlugin.Publish(suber.StreamPath, &suber.BatchUplink)
				}
				if suber.Publisher.Stream == nil {
					return
				}
				puber := &suber.Publisher
				if codec := track.Codec(); track.Kind() == RTPCodecTypeAudio {
					if puber.AudioTrack == nil {
						switch codec.PayloadType {
						case 8:
							puber.AudioTrack = NewG711(puber.Stream, true)
						case 0:
							puber.AudioTrack = NewG711(puber.Stream, false)
						default:
							puber.AudioTrack = nil
							return
						}
					}
					for {
						b := make([]byte, 1460)
						if i, _, err := track.Read(b); err == nil {
							puber.AudioTrack.WriteRTP(b[:i])
						} else {
							return
						}
					}
				} else {
					go func() {
						ticker := time.NewTicker(webrtcConfig.PLI)
						for {
							select {
							case <-ticker.C:
								if rtcpErr := suber.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}}); rtcpErr != nil {
									fmt.Println(rtcpErr)
								}
							case <-puber.Done():
								return
							}
						}
					}()
					puber.VideoTrack = NewH264(puber.Stream, byte(codec.PayloadType))
					for {
						b := make([]byte, 1460)
						if i, _, err := track.Read(b); err == nil {
							puber.VideoTrack.WriteRTP(b[:i])
						} else {
							return
						}
					}
				}
			})
		case PeerConnectionStateDisconnected, PeerConnectionStateFailed:
			for _, sub := range suber.subscribers {
				go sub.Stop()
			}
			suber.PeerConnection.Close()
		}
	})
	return
}

func (suber *WebRTCBatcher) Signal(msg DataChannelMessage) {
	var s Signal
	var removeMap = map[string]string{"type": "remove", "streamPath": ""}
	// var offer SessionDescription
	if err := json.Unmarshal(msg.Data, &s); err != nil {
		WebRTCPlugin.Error("Signal", zap.Error(err))
	} else {
		switch s.Type {
		case "streamPath":
			suber.StreamPath = s.StreamPath
		case "subscribe":
			if err = suber.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: s.Offer}); err != nil {
				WebRTCPlugin.Error("Signal SetRemoteDescription", zap.Error(err))
				return
			}
			for _, streamPath := range s.StreamList {
				sub := &WebRTCBatchSubscriber{}
				sub.WebRTCIO = suber.WebRTCIO
				if err = WebRTCPlugin.SubscribeExist(streamPath, sub); err == nil {
					suber.subscribers = append(suber.subscribers, sub)
					go func() {
						sub.PlayRTP()
						if sub.audioSender != nil {
							suber.RemoveTrack(sub.audioSender)
						}
						if sub.videoSender != nil {
							suber.RemoveTrack(sub.videoSender)
						}
						if sub.DC != nil {
							sub.DC.Close()
						}
						removeMap["streamPath"] = streamPath
						b, _ := json.Marshal(removeMap)
						suber.signalChannel.SendText(string(b))
					}()
				} else {
					removeMap["streamPath"] = streamPath
					b, _ := json.Marshal(removeMap)
					suber.signalChannel.SendText(string(b))
				}
			}
			var answer string
			if answer, err = suber.GetAnswer(); err == nil {
				b, _ := json.Marshal(map[string]string{"type": "answer", "sdp": answer})
				err = suber.signalChannel.SendText(string(b))
			}
			if err != nil {
				WebRTCPlugin.Error("Signal GetAnswer", zap.Error(err))
				return
			}
		// if offer, err = suber.CreateOffer(nil); err == nil {
		// 	b, _ := json.Marshal(offer)
		// 	err = suber.signalChannel.SendText(string(b))
		// 	suber.SetLocalDescription(offer)
		// }
		case "publish":
			if err = suber.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: s.Offer}); err != nil {
				WebRTCPlugin.Error("Signal SetRemoteDescription", zap.Error(err))
				return
			}
			var answer string
			if answer, err = suber.GetAnswer(); err == nil {
				b, _ := json.Marshal(map[string]string{"type": "answer", "sdp": answer})
				err = suber.signalChannel.SendText(string(b))
			}
			if err != nil {
				WebRTCPlugin.Error("Signal GetAnswer", zap.Error(err))
				return
			}
		case "answer":
			if err = suber.SetRemoteDescription(SessionDescription{Type: SDPTypeAnswer, SDP: s.Answer}); err != nil {
				WebRTCPlugin.Error("Signal SetRemoteDescription", zap.Error(err))
				return
			}
		}
	}
}
