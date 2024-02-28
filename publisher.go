package webrtc

import (
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v4"
	"go.uber.org/zap"
	. "m7s.live/engine/v4"
	"m7s.live/engine/v4/codec"
)

type WebRTCPublisher struct {
	Publisher
	WebRTCIO
	audioTrack atomic.Pointer[TrackRemote]
	videoTrack atomic.Pointer[TrackRemote]
}

func (puber *WebRTCPublisher) OnEvent(event any) {
	switch event.(type) {
	case IPublisher:
		puber.OnTrack(puber.onTrack)
	}
	puber.Publisher.OnEvent(event)
}

func (puber *WebRTCPublisher) onTrack(track *TrackRemote, receiver *RTPReceiver) {
	puber.Info("onTrack", zap.String("kind", track.Kind().String()), zap.Uint8("payloadType", uint8(track.Codec().PayloadType)))
	if codecP := track.Codec(); track.Kind() == RTPCodecTypeAudio {
		puber.audioTrack.Store(track)
		if puber.AudioTrack == nil {
			switch codecP.PayloadType {
			case 111:
				puber.CreateAudioTrack(codec.CodecID_OPUS)
			case 8:
				puber.CreateAudioTrack(codec.CodecID_PCMA)
			case 0:
				puber.CreateAudioTrack(codec.CodecID_PCMU)
			default:
				puber.AudioTrack = nil
				puber.Config.PubAudio = false
				return
			}
		}
		for {
			if puber.audioTrack.Load() != track {
				return
			}
			rtpItem := puber.AudioTrack.GetRTPFromPool()
			if i, _, err := track.Read(rtpItem.Value.Raw); err == nil {
				rtpItem.Value.Unmarshal(rtpItem.Value.Raw[:i])
				puber.AudioTrack.WriteRTP(rtpItem)
			} else {
				puber.Info("track stop", zap.String("kind", track.Kind().String()), zap.Error(err))
				rtpItem.Recycle()
				return
			}
		}
	} else {
		puber.videoTrack.Store(track)
		if puber.VideoTrack == nil {
			switch codecP.PayloadType {
			case 45:
				puber.CreateVideoTrack(codec.CodecID_AV1, byte(codecP.PayloadType))
			default:
				puber.CreateVideoTrack(codec.CodecID_H264, byte(codecP.PayloadType))
			}
		}
		go puber.writeRTCP(track)
		for {
			if puber.videoTrack.Load() != track {
				return
			}
			rtpItem := puber.VideoTrack.GetRTPFromPool()
			if i, _, err := track.Read(rtpItem.Value.Raw); err == nil {
				rtpItem.Value.Unmarshal(rtpItem.Value.Raw[:i])
				if rtpItem.Value.Extension {
					for _, id := range rtpItem.Value.GetExtensionIDs() {
						puber.Debug("extension", zap.Uint8("id", id), zap.Binary("value", rtpItem.Value.GetExtension(id)))
					}
				}
				puber.VideoTrack.WriteRTP(rtpItem)
			} else {
				puber.Info("track stop", zap.String("kind", track.Kind().String()), zap.Error(err))
				rtpItem.Recycle()
				return
			}
		}
	}
}

func (puber *WebRTCPublisher) writeRTCP(track *TrackRemote) {
	ticker := time.NewTicker(webrtcConfig.PLI)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if puber.videoTrack.Load() != track {
				return
			}
			if rtcpErr := puber.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}}); rtcpErr != nil {
				puber.Error("writeRTCP", zap.Error(rtcpErr))
				return
			}
		case <-puber.Done():
			return
		}
	}
}
