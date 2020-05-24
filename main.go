package webrtc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/Monibuca/engine/v2"
	rtsp "github.com/Monibuca/plugin-rtsp"
	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v2"
)

var config = &struct {
	ICEServers []string
}{[]string{
	"stun:stun.ekiga.net",
	"stun:stun.ideasip.com",
	"stun:stun.schlund.de",
	"stun:stun.stunprotocol.org:3478",
	"stun:stun.voiparound.com",
	"stun:stun.voipbuster.com",
	"stun:stun.voipstunt.com",
	"stun:stun.voxgratia.org",
	"stun:stun.services.mozilla.com",
	"stun:stun.xten.com",
	"stun:stun.softjoys.com",
	"stun:stunserver.org",
	"stun:stun.schlund.de",
	"stun:stun.rixtelecom.se",
	"stun:stun.iptel.org",
	"stun:stun.ideasip.com",
	"stun:stun.fwdnet.net",
	"stun:stun.ekiga.net",
	"stun:stun01.sipphone.com",
}}

// type udpConn struct {
// 	conn *net.UDPConn
// 	port int
// }

var m = MediaEngine{}
var api *API

func init() {
	m.RegisterCodec(NewRTPH264Codec(DefaultPayloadTypeH264, 90000))
	//m.RegisterCodec(NewRTPPCMUCodec(DefaultPayloadTypePCMU, 8000))
	api = NewAPI(WithMediaEngine(m))
	InstallPlugin(&PluginConfig{
		Config: config,
		Name:   "WebRTC",
		Type:   PLUGIN_PUBLISHER | PLUGIN_SUBSCRIBER,
		Run:    run,
	})
}

type WebRTC struct {
	rtsp.RTSP
	*PeerConnection
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
		fmt.Printf("Connection State has changed %s \n", connectionState.String())

		if connectionState == ICEConnectionStateConnected {
			fmt.Println("Ctrl+C the remote client to stop the demo")
		} else if connectionState == ICEConnectionStateFailed ||
			connectionState == ICEConnectionStateDisconnected {
			fmt.Println("Done forwarding")
			//TODO
		}
	})
	rtc.PeerConnection = peerConnection
	if rtc.RTSP.Publish(streamPath) {
		rtc.Stream.Type = "WebRTC"
		peerConnection.OnTrack(func(track *Track, receiver *RTPReceiver) {
			defer rtc.Close()
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
			pack := &rtsp.RTPPack{
				Type: rtsp.RTPType(track.Kind() - 1),
			}
			for b := make([]byte, 1460); ; rtc.HandleRTP(pack) {
				i, err := track.Read(b)
				if err != nil {
					return
				}
				if err = pack.Unmarshal(b[:i]); err != nil {
					return
				}
			}
		})
	} else {
		return false
	}
	return true
}
func run() {
	http.HandleFunc("/webrtc/answer", func(w http.ResponseWriter, r *http.Request) {
		streamPath := r.URL.Query().Get("streamPath")
		offer := SessionDescription{}
		bytes, err := ioutil.ReadAll(r.Body)
		err = json.Unmarshal(bytes, &offer)
		if err != nil {
			Println(err)
			return
		}
		rtc := new(WebRTC)
		if rtc.Publish(streamPath) {
			// Set the remote SessionDescription
			if err = rtc.SetRemoteDescription(offer); err != nil {
				Println(err)
				return
			}

			// Create answer
			answer, err := rtc.CreateAnswer(nil)
			if err != nil {
				Println(err)
				return
			}

			// Sets the LocalDescription, and starts our UDP listeners
			if err = rtc.SetLocalDescription(answer); err != nil {
				Println(err)
				return
			}
			if bytes, err = json.Marshal(answer); err != nil {
				Println(err)
				return
			}
			w.Write(bytes)
		}
	})
}
