package webrtc

import (
	"fmt"
	"net"
	"strings"

	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v3"
	"go.uber.org/zap"
	. "m7s.live/engine/v4"
	"m7s.live/engine/v4/codec"
	"m7s.live/engine/v4/track"
	"m7s.live/engine/v4/util"
)

type WebRTCSubscriber struct {
	Subscriber
	WebRTCIO
	videoTrack   *TrackLocalStaticRTP
	audioTrack   *TrackLocalStaticRTP
	videoSender  *RTPSender
	audioSender  *RTPSender
	DC           *DataChannel
	flvHeadCache []byte
}

func (suber *WebRTCSubscriber) createDataChannel() {
	if suber.DC != nil {
		return
	}
	suber.DC, _ = suber.PeerConnection.CreateDataChannel(suber.Subscriber.Stream.Path, nil)
	suber.flvHeadCache = make([]byte, 15)
	suber.DC.Send(codec.FLVHeader)
}
func (suber *WebRTCSubscriber) sendAvByDatachannel(t byte, reader *track.AVRingReader) {
	suber.flvHeadCache[0] = t
	frame := reader.Frame
	dataSize := uint32(frame.AVCC.ByteLength)
	result := net.Buffers{suber.flvHeadCache[:11]}
	result = append(result, frame.AVCC.ToBuffers()...)
	ts := reader.AbsTime
	util.PutBE(suber.flvHeadCache[1:4], dataSize)
	util.PutBE(suber.flvHeadCache[4:7], ts)
	suber.flvHeadCache[7] = byte(ts >> 24)
	result = append(result, util.PutBE(suber.flvHeadCache[11:15], dataSize+11))
	for _, data := range util.SplitBuffers(result, 65535) {
		for _, d := range data {
			suber.DC.Send(d)
		}
	}
}

func (suber *WebRTCSubscriber) OnEvent(event any) {
	switch v := event.(type) {
	case *track.Video:
		switch v.CodecID {
		case codec.CodecID_H264:
			pli := fmt.Sprintf("%x", v.SPS[1:4])
			// pli := "42001f"
			if !strings.Contains(suber.SDP, pli) {
				list := reg_level.FindAllStringSubmatch(suber.SDP, -1)
				if len(list) > 0 {
					pli = list[0][1]
				}
			}
			suber.videoTrack, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: MimeTypeH264, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + pli}, v.Name, suber.Subscriber.Stream.Path)
		case codec.CodecID_H265:
			// suber.videoTrack, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: MimeTypeH265, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + pli}, "video", suber.Subscriber.Stream.Path)
		default:
			return
		}
		if suber.videoTrack == nil {
			suber.createDataChannel()
		} else {
			suber.videoSender, _ = suber.PeerConnection.AddTrack(suber.videoTrack)
			go func() {
				rtcpBuf := make([]byte, 1500)
				for {
					if n, _, rtcpErr := suber.videoSender.Read(rtcpBuf); rtcpErr != nil {

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
		suber.Subscriber.AddTrack(v) //接受这个track
	case *track.Audio:
		audioMimeType := MimeTypePCMA
		if v.CodecID == codec.CodecID_PCMU {
			audioMimeType = MimeTypePCMU
		}
		switch v.CodecID {
		case codec.CodecID_AAC:
			suber.createDataChannel()
		case codec.CodecID_PCMA, codec.CodecID_PCMU:
			suber.audioTrack, _ = NewTrackLocalStaticRTP(RTPCodecCapability{MimeType: audioMimeType}, v.Name, suber.Subscriber.Stream.Path)
			suber.audioSender, _ = suber.PeerConnection.AddTrack(suber.audioTrack)
			suber.Subscriber.AddTrack(v) //接受这个track
		}
	case VideoDeConf:
		if suber.DC != nil {
			suber.DC.Send(util.ConcatBuffers(codec.VideoAVCC2FLV(0, v)))
		}
	case AudioDeConf:
		if suber.DC != nil {
			suber.DC.Send(util.ConcatBuffers(codec.AudioAVCC2FLV(0, v)))
		}
	case VideoRTP:
		if suber.videoTrack != nil {
			suber.Trace("video rtp", zap.Any("packet", v.Packet.Header))
			suber.videoTrack.WriteRTP(v.Packet)
		} else if suber.DC != nil {
			suber.sendAvByDatachannel(9, &suber.VideoReader)
		}
	case AudioRTP:
		if suber.audioTrack != nil {
			suber.audioTrack.WriteRTP(v.Packet)
		} else if suber.DC != nil {
			suber.sendAvByDatachannel(8, &suber.AudioReader)
		}
	case ISubscriber:
		suber.OnConnectionStateChange(func(pcs PeerConnectionState) {
			suber.Info("Connection State has changed:" + pcs.String())
			switch pcs {
			case PeerConnectionStateConnected:
				go suber.PlayRTP()
			case PeerConnectionStateDisconnected, PeerConnectionStateFailed:
				suber.Stop()
				suber.PeerConnection.Close()
			}
		})
	default:
		suber.Subscriber.OnEvent(event)
	}
}

type WebRTCBatchSubscriber struct {
	WebRTCSubscriber
}

func (suber *WebRTCBatchSubscriber) OnEvent(event any) {
	switch event.(type) {
	case ISubscriber:
	default:
		suber.WebRTCSubscriber.OnEvent(event)
	}
}
