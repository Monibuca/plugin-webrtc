package webrtc

import (
	"encoding/json"
	"fmt"

	. "github.com/pion/webrtc/v3"
	"go.uber.org/zap"
	"m7s.live/engine/v4/codec"
	"m7s.live/engine/v4/util"
)

type Signal struct {
	Type       string   `json:"type"`
	StreamList []string `json:"streamList"`
	Offer      string   `json:"offer"`
	Answer     string   `json:"answer"`
	StreamPath string   `json:"streamPath"`
}

type SignalStreamPath struct {
	Type       string `json:"type"`
	StreamPath string `json:"streamPath"`
}

func NewRemoveSingal(streamPath string) string {
	s := SignalStreamPath{
		Type:       "remove",
		StreamPath: streamPath,
	}
	b, _ := json.Marshal(s)
	return string(b)
}

type SignalSDP struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

func NewAnswerSingal(sdp string) string {
	s := SignalSDP{
		Type: "answer",
		SDP:  sdp,
	}
	b, _ := json.Marshal(s)
	return string(b)
}

type WebRTCBatcher struct {
	PageSize      int
	PageNum       int
	subscribers   util.Map[string,*WebRTCBatchSubscriber]
	signalChannel *DataChannel
	WebRTCPublisher
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

		case PeerConnectionStateDisconnected, PeerConnectionStateFailed:
			zr := zap.String("reason", pcs.String())
			suber.subscribers.Range(func(key string, value *WebRTCBatchSubscriber) {
				value.Stop(zr)
			})
			if suber.Publisher.Stream != nil {
				suber.Publisher.Stop(zr)
			}
			suber.PeerConnection.Close()
		}
	})
	return
}

func (suber *WebRTCBatcher) RemoveSubscribe(streamPath string) {
	suber.signalChannel.SendText(NewRemoveSingal(streamPath))
}
func (suber *WebRTCBatcher) Answer() (err error) {
	var answer string
	if answer, err = suber.GetAnswer(); err == nil {
		err = suber.signalChannel.SendText(NewAnswerSingal(answer))
	}
	if err != nil {
		WebRTCPlugin.Error("Signal GetAnswer", zap.Error(err))
	}
	return
}

func (suber *WebRTCBatcher) Signal(msg DataChannelMessage) {
	var s Signal
	// var offer SessionDescription
	if err := json.Unmarshal(msg.Data, &s); err != nil {
		WebRTCPlugin.Error("Signal", zap.Error(err))
	} else {
		switch s.Type {
		case "subscribe":
			if err = suber.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: s.Offer}); err != nil {
				WebRTCPlugin.Error("Signal SetRemoteDescription", zap.Error(err))
				return
			}
			for _, streamPath := range s.StreamList {
				if suber.subscribers.Has(streamPath) {
					continue
				}
				sub := &WebRTCBatchSubscriber{}
				sub.ID = fmt.Sprintf("%s_%s", suber.ID, streamPath)
				sub.WebRTCIO = suber.WebRTCIO
				if err = WebRTCPlugin.SubscribeExist(streamPath, sub); err == nil {
					suber.subscribers.Add(streamPath, sub)
					go func(streamPath string) {
						if sub.DC == nil {
							sub.PlayRTP()
							if sub.audio.RTPSender != nil {
								suber.RemoveTrack(sub.audio.RTPSender)
							}
							if sub.video.RTPSender != nil {
								suber.RemoveTrack(sub.video.RTPSender)
							}
							suber.RemoveSubscribe(streamPath)
						} else {
							sub.DC.OnOpen(func() {
								sub.DC.Send(codec.FLVHeader)
								go func() {
									sub.PlayFLV()
									sub.DC.Close()
									suber.RemoveSubscribe(streamPath)
								}()
							})
						}
					}(streamPath)
				} else {
					WebRTCPlugin.Error("subscribe", zap.String("streamPath", streamPath), zap.Error(err))
					suber.RemoveSubscribe(streamPath)
				}
			}
			err = suber.Answer()
		// if offer, err = suber.CreateOffer(nil); err == nil {
		// 	b, _ := json.Marshal(offer)
		// 	err = suber.signalChannel.SendText(string(b))
		// 	suber.SetLocalDescription(offer)
		// }
		case "publish", "unpublish":
			if err = suber.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: s.Offer}); err != nil {
				WebRTCPlugin.Error("Signal SetRemoteDescription", zap.Error(err))
				return
			}
			if err = suber.Answer(); err == nil {
				switch s.Type {
				case "publish":
					WebRTCPlugin.Publish(s.StreamPath, suber)
				case "unpublish":
					suber.Stop()
				}
			}
		case "answer":
			if err = suber.SetRemoteDescription(SessionDescription{Type: SDPTypeAnswer, SDP: s.Answer}); err != nil {
				WebRTCPlugin.Error("Signal SetRemoteDescription", zap.Error(err))
				return
			}
		}
		WebRTCPlugin.Info(s.Type)
	}
}
