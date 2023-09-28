package webrtc

import (
	"fmt"
	"strings"

	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v3"
	"go.uber.org/zap"
	. "m7s.live/engine/v4"
	"m7s.live/engine/v4/codec"
	"m7s.live/engine/v4/track"
	"m7s.live/engine/v4/util"
)

type trackSender struct {
	*TrackLocalStaticRTP
	*RTPSender
	// seq uint32
}

type WebRTCSubscriber struct {
	Subscriber
	WebRTCIO
	audio       trackSender
	video       trackSender
	DC          *DataChannel
	videoTracks []*track.Video
	audioTracks []*track.Audio
	// flvHeadCache []byte
}

func (suber *WebRTCSubscriber) queueDCData(data ...[]byte) (err error) {
	for _, d := range data {
		if err = suber.DC.Send(d); err != nil {
			return
		}
	}
	return
}

func (suber *WebRTCSubscriber) createDataChannel() {
	if suber.DC != nil {
		return
	}
	suber.DC, _ = suber.PeerConnection.CreateDataChannel(suber.Subscriber.Stream.Path, nil)
	// suber.flvHeadCache = make([]byte, 15)
}

//	func (suber *WebRTCSubscriber) sendAvByDatachannel(t byte, reader *track.AVRingReader) {
//		suber.flvHeadCache[0] = t
//		frame := reader.Frame
//		dataSize := uint32(frame.AVCC.ByteLength)
//		result := net.Buffers{suber.flvHeadCache[:11]}
//		result = append(result, frame.AVCC.ToBuffers()...)
//		ts := reader.AbsTime
//		util.PutBE(suber.flvHeadCache[1:4], dataSize)
//		util.PutBE(suber.flvHeadCache[4:7], ts)
//		suber.flvHeadCache[7] = byte(ts >> 24)
//		result = append(result, util.PutBE(suber.flvHeadCache[11:15], dataSize+11))
//		for _, data := range util.SplitBuffers(result, 65535) {
//			for _, d := range data {
//				suber.queueDCData(d)
//			}
//		}
//	}
func (suber *WebRTCSubscriber) OnSubscribe() {
	vm := make(map[codec.VideoCodecID]*track.Video)
	am := make(map[codec.AudioCodecID]*track.Audio)
	for _, track := range suber.videoTracks {
		vm[track.CodecID] = track
	}
	for _, track := range suber.audioTracks {
		am[track.CodecID] = track
	}
	if (vm[codec.CodecID_H264] != nil || vm[codec.CodecID_H265] == nil) && (am[codec.CodecID_PCMA] != nil || am[codec.CodecID_PCMU] != nil || am[codec.CodecID_AAC] == nil) {
		video := vm[codec.CodecID_H264]
		if video != nil {
			suber.Subscriber.AddTrack(video)
			pli := fmt.Sprintf("%x", video.SPS[1:4])
			// pli := "42001f"
			if !strings.Contains(suber.SDP, pli) {
				list := reg_level.FindAllStringSubmatch(suber.SDP, -1)
				if len(list) > 0 {
					pli = list[0][1]
				}
			}
			suber.video.TrackLocalStaticRTP, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: MimeTypeH264, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + pli}, video.Name, suber.Subscriber.Stream.Path)
			if suber.video.TrackLocalStaticRTP != nil {
				suber.video.RTPSender, _ = suber.PeerConnection.AddTrack(suber.video.TrackLocalStaticRTP)
				go func() {
					rtcpBuf := make([]byte, 1500)
					for {
						if n, _, rtcpErr := suber.video.Read(rtcpBuf); rtcpErr != nil {
							suber.Warn("rtcp read error", zap.Error(rtcpErr))
							return
						} else {
							if p, err := rtcp.Unmarshal(rtcpBuf[:n]); err == nil {
								for _, pp := range p {
									switch pp.(type) {
									case *rtcp.PictureLossIndication:
										// fmt.Println("PictureLossIndication")
									}
								}
							}
						}
					}
				}()
			}
		}
		var audio *track.Audio
		audioMimeType := MimeTypePCMA
		if am[codec.CodecID_PCMA] != nil {
			audio = am[codec.CodecID_PCMA]
		} else if am[codec.CodecID_PCMU] != nil {
			audioMimeType = MimeTypePCMU
			audio = am[codec.CodecID_PCMU]
		} else {

		}
		if audio != nil {
			suber.Subscriber.AddTrack(audio)
			suber.audio.TrackLocalStaticRTP, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: audioMimeType}, audio.Name, suber.Subscriber.Stream.Path)
			if suber.audio.TrackLocalStaticRTP != nil {
				suber.audio.RTPSender, _ = suber.PeerConnection.AddTrack(suber.audio.TrackLocalStaticRTP)
			}
		}
	} else {
		suber.createDataChannel()
		if len(suber.videoTracks) > 0 {
			suber.Subscriber.AddTrack(suber.videoTracks[0])
		}
		if len(suber.audioTracks) > 0 {
			suber.Subscriber.AddTrack(suber.audioTracks[0])
		}

	}
}

func (suber *WebRTCSubscriber) OnEvent(event any) {
	var err error
	switch v := event.(type) {
	case *track.Video:
		suber.videoTracks = append(suber.videoTracks, v)
		// switch v.CodecID {
		// case codec.CodecID_H264:
		// 	pli := fmt.Sprintf("%x", v.SPS[1:4])
		// 	// pli := "42001f"
		// 	if !strings.Contains(suber.SDP, pli) {
		// 		list := reg_level.FindAllStringSubmatch(suber.SDP, -1)
		// 		if len(list) > 0 {
		// 			pli = list[0][1]
		// 		}
		// 	}
		// 	suber.video.TrackLocalStaticRTP, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: MimeTypeH264, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + pli}, v.Name, suber.Subscriber.Stream.Path)
		// case codec.CodecID_H265:
		// 	suber.createDataChannel()
		// 	// suber.videoTrack, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: MimeTypeH265, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + pli}, "video", suber.Subscriber.Stream.Path)
		// default:
		// 	return
		// }
		// suber.Subscriber.AddTrack(v) //接受这个track
	case *track.Audio:
		// audioMimeType := MimeTypePCMA
		// if v.CodecID == codec.CodecID_PCMU {
		// 	audioMimeType = MimeTypePCMU
		// }
		// switch v.CodecID {
		// case codec.CodecID_AAC:
		// 	suber.createDataChannel()
		// case codec.CodecID_PCMA, codec.CodecID_PCMU:
		// 	suber.audio.TrackLocalStaticRTP, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: audioMimeType}, v.Name, suber.Subscriber.Stream.Path)
		// 	//suber.audio.RTPSender, _ = suber.PeerConnection.AddTrack(suber.audio.TrackLocalStaticRTP)
		// }
		// suber.Subscriber.AddTrack(v) //接受这个track
		suber.audioTracks = append(suber.audioTracks, v)
	// case VideoDeConf:
	// 	if suber.DC != nil {
	// 		suber.queueDCData(codec.VideoAVCC2FLV(0, v)...)
	// 	}
	// case AudioDeConf:
	// 	if suber.DC != nil {
	// 		suber.queueDCData(codec.AudioAVCC2FLV(0, v)...)
	// 	}
	case VideoRTP:
		// if suber.video.TrackLocalStaticRTP != nil {
		if err = suber.video.WriteRTP(v.Packet); err != nil {
			suber.Stop(zap.Error(err))
			return
		}
		// } else if suber.DC != nil && suber.VideoReader.Frame.Sequence != suber.video.seq {
		// 	suber.video.seq = suber.VideoReader.Frame.Sequence
		// 	suber.sendAvByDatachannel(9, suber.VideoReader)
		// }
	case AudioRTP:
		// if suber.audio.TrackLocalStaticRTP != nil {
		if err = suber.audio.WriteRTP(v.Packet); err != nil {
			suber.Stop(zap.Error(err))
			return
		}
		// } else if suber.DC != nil && suber.AudioReader.Frame.Sequence != suber.audio.seq {
		// 	suber.audio.seq = suber.AudioReader.Frame.Sequence
		// 	suber.sendAvByDatachannel(8, suber.AudioReader)
		// }
	case FLVFrame:
		for _, data := range util.SplitBuffers(v, 65535) {
			if err = suber.queueDCData(data...); err != nil {
				suber.Stop(zap.Error(err))
				return
			}
		}
	case ISubscriber:
		suber.OnSubscribe()
		if suber.DC != nil {
			suber.DC.OnOpen(func() {
				suber.DC.Send(codec.FLVHeader)
				go func() {
					suber.PlayFLV()
					suber.DC.Close()
					suber.PeerConnection.Close()
				}()
			})
		}
		suber.OnConnectionStateChange(func(pcs PeerConnectionState) {
			suber.Info("Connection State has changed:" + pcs.String())
			switch pcs {
			case PeerConnectionStateConnected:
				if suber.DC == nil {
					go func() {
						suber.PlayRTP()
						suber.PeerConnection.Close()
					}()
				}
			case PeerConnectionStateDisconnected, PeerConnectionStateFailed:
				suber.Stop(zap.String("reason", pcs.String()))
			}
		})
	default:
		suber.Subscriber.OnEvent(event)
	}
}

type WebRTCBatchSubscriber struct {
	WebRTCSubscriber
	OnPlayDone func()
}

func (suber *WebRTCBatchSubscriber) OnEvent(event any) {
	switch event.(type) {
	case ISubscriber:
		suber.OnSubscribe()
	default:
		suber.WebRTCSubscriber.OnEvent(event)
	}
}
