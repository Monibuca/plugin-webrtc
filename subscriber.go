package webrtc

import (
	"fmt"
	"strings"

	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v3"
	. "m7s.live/engine/v4"
	"m7s.live/engine/v4/codec"
	"m7s.live/engine/v4/track"
)

type WebRTCSubscriber struct {
	Subscriber
	WebRTCIO
	videoTrack *TrackLocalStaticRTP
	audioTrack *TrackLocalStaticRTP
}

func (suber *WebRTCSubscriber) OnEvent(event any) {
	switch v := event.(type) {
	case *track.Video:
		if v.CodecID == codec.CodecID_H264 {
			pli := "42001f"
			pli = fmt.Sprintf("%x", v.GetDecoderConfiguration().Raw[0][1:4])
			if !strings.Contains(suber.SDP, pli) {
				pli = reg_level.FindAllStringSubmatch(suber.SDP, -1)[0][1]
			}
			suber.videoTrack, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: MimeTypeH264, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + pli}, "video", "m7s")
			rtpSender, _ := suber.PeerConnection.AddTrack(suber.videoTrack)
			go func() {
				rtcpBuf := make([]byte, 1500)
				for {
					if n, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {

						return
					} else {
						if p, err := rtcp.Unmarshal(rtcpBuf[:n]); err == nil {
							for _, pp := range p {
								switch pp.(type) {
								case *rtcp.PictureLossIndication:

									fmt.Println("PictureLossIndication")
								}
							}
						}
					}
				}
			}()
			suber.Subscriber.AddTrack(v) //接受这个track
		}
	case *track.Audio:
		audioMimeType := MimeTypePCMA
		if v.CodecID == codec.CodecID_PCMU {
			audioMimeType = MimeTypePCMU
		}
		if v.CodecID == codec.CodecID_PCMA || v.CodecID == codec.CodecID_PCMU {
			suber.audioTrack, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: audioMimeType}, "audio", "m7s")
			suber.PeerConnection.AddTrack(suber.audioTrack)
			suber.Subscriber.AddTrack(v) //接受这个track
		}
	case *VideoFrame:
		for _, p := range v.RTP {
			suber.videoTrack.Write(p.Raw)
		}
	case *AudioFrame:
		for _, p := range v.RTP {
			suber.audioTrack.Write(p.Raw)
		}
	case ISubscriber:
		suber.OnConnectionStateChange(func(pcs PeerConnectionState) {
			suber.Info("Connection State has changed:" + pcs.String())
			switch pcs {
			case PeerConnectionStateConnected:
				go suber.PlayBlock()
			case PeerConnectionStateDisconnected, PeerConnectionStateFailed:
				suber.Stop()
				suber.PeerConnection.Close()
			}
		})
	default:
		suber.Subscriber.OnEvent(event)
	}
}
