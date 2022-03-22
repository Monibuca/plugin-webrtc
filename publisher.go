package webrtc

import (
	"fmt"
	"time"

	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v3"
	. "m7s.live/engine/v4"
	. "m7s.live/engine/v4/track"
)

type WebRTCPublisher struct {
	Publisher
	WebRTCIO
}

func (puber *WebRTCPublisher) OnEvent(event any) {
	switch v := event.(type) {
	case IPublisher:
		puber.OnICEConnectionStateChange(func(connectionState ICEConnectionState) {
			puber.Info("Connection State has changed:" + connectionState.String())
			switch connectionState {
			case ICEConnectionStateDisconnected, ICEConnectionStateFailed:
				puber.Stop()
			}
		})
		puber.OnTrack(func(track *TrackRemote, receiver *RTPReceiver) {
			if codec := track.Codec(); track.Kind() == RTPCodecTypeAudio {
				if puber.Equal(v) || puber.AudioTrack == nil {
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
					ticker := time.NewTicker(time.Millisecond * webrtcConfig.PLI)
					for {
						select {
						case <-ticker.C:
							if rtcpErr := puber.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}}); rtcpErr != nil {
								fmt.Println(rtcpErr)
							}
						case <-puber.Done():
							return
						}
					}
				}()
				if puber.Equal(v) {
					puber.VideoTrack = NewH264(puber.Stream)
				}
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
	}
	puber.Publisher.OnEvent(event)
}
