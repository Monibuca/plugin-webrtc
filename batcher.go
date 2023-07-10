package webrtc

import (
	"encoding/json"

	. "github.com/pion/webrtc/v3"
	"go.uber.org/zap"
	"m7s.live/engine/v4/codec"
)

type Signal struct {
	Type       string   `json:"type"`
	StreamList []string `json:"streamList"`
	Offer      string   `json:"offer"`
	Answer     string   `json:"answer"`
	StreamPath string   `json:"streamPath"`
}

type WebRTCBatcher struct {
	PageSize      int
	PageNum       int
	subscribers   []*WebRTCBatchSubscriber
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
			for _, sub := range suber.subscribers {
				go sub.Stop()
			}
			if suber.Publisher.Stream != nil {
				suber.Publisher.Stop()
			}
			suber.PeerConnection.Close()
		}
	})
	return
}

func (suber *WebRTCBatcher) RemoveSubscribe(streamPath string) {
	b, _ := json.Marshal(map[string]string{"type": "remove", "streamPath": streamPath})
	suber.signalChannel.SendText(string(b))
}

func (suber *WebRTCBatcher) Signal(msg DataChannelMessage) {
	var s Signal
	var removeMap = map[string]string{"type": "remove", "streamPath": ""}
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
				sub := &WebRTCBatchSubscriber{}
				sub.WebRTCIO = suber.WebRTCIO
				if err = WebRTCPlugin.SubscribeExist(streamPath, sub); err == nil {
					suber.subscribers = append(suber.subscribers, sub)
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
		case "publish", "unpublish":
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
			switch s.Type {
			case "publish":
				WebRTCPlugin.Publish(s.StreamPath, suber)
			case "unpublish":
				suber.Stop()
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
