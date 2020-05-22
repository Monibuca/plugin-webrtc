package webrtc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	. "github.com/Monibuca/engine/v2"
	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v2"
)

var config = &struct {
	ICEServers []string
}{[]string{
	"stun.l.google.com:19302",
	"stun1.l.google.com:19302",
	"stun2.l.google.com:19302",
	"stun3.l.google.com:19302",
	"stun4.l.google.com:19302",
	"stun.ekiga.net",
	"stun.ideasip.com",
	"stun.schlund.de",
	"stun.stunprotocol.org:3478",
	"stun.voiparound.com",
	"stun.voipbuster.com",
	"stun.voipstunt.com",
	"stun.voxgratia.org",
	"stun.services.mozilla.com",
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

type udpConn struct {
	conn *net.UDPConn
	port int
}

func init() {
	InstallPlugin(&PluginConfig{
		Config: config,
		Name:   "WebRTC",
		Type:   PLUGIN_PUBLISHER | PLUGIN_SUBSCRIBER,
		Run:    run,
	})
}
func run() {
	m := MediaEngine{}
	m.RegisterCodec(NewRTPH264Codec(DefaultPayloadTypeH264, 90000))
	api := NewAPI(WithMediaEngine(m))
	peerConnection, err := api.NewPeerConnection(Configuration{
		ICEServers: []ICEServer{
			{
				URLs: config.ICEServers,
			},
		},
	})
	if err != nil {
		Println(err)
		return
	}
	// Allow us to receive 1 audio track, and 1 video track
	if _, err = peerConnection.AddTransceiverFromKind(RTPCodecTypeAudio); err != nil {
		if err != nil {
			Println(err)
			return
		}
	} else if _, err = peerConnection.AddTransceiverFromKind(RTPCodecTypeVideo); err != nil {
		if err != nil {
			Println(err)
			return
		}
	}
	var laddr *net.UDPAddr
	if laddr, err = net.ResolveUDPAddr("udp", "127.0.0.1:"); err != nil {
		panic(err)
	}

	// Prepare udp conns
	udpConns := map[string]*udpConn{
		"audio": {port: 4000},
		"video": {port: 4002},
	}
	for _, c := range udpConns {
		// Create remote addr
		var raddr *net.UDPAddr
		if raddr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", c.port)); err != nil {
			panic(err)
		}

		// Dial udp
		if c.conn, err = net.DialUDP("udp", laddr, raddr); err != nil {
			panic(err)
		}
		defer func(conn net.PacketConn) {
			if closeErr := conn.Close(); closeErr != nil {
				panic(closeErr)
			}
		}(c.conn)
	}

	// Set a handler for when a new remote track starts, this handler will forward data to
	// our UDP listeners.
	// In your application this is where you would handle/process audio/video
	peerConnection.OnTrack(func(track *Track, receiver *RTPReceiver) {
		// Retrieve udp connection
		c, ok := udpConns[track.Kind().String()]
		if !ok {
			return
		}

		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		go func() {
			ticker := time.NewTicker(time.Second * 2)
			for range ticker.C {
				if rtcpErr := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: track.SSRC()}}); rtcpErr != nil {
					fmt.Println(rtcpErr)
				}
			}
		}()

		b := make([]byte, 1500)
		for {
			// Read
			n, readErr := track.Read(b)
			if readErr != nil {
				panic(readErr)
			}

			// Write
			if _, err = c.conn.Write(b[:n]); err != nil {
				// For this particular example, third party applications usually timeout after a short
				// amount of time during which the user doesn't have enough time to provide the answer
				// to the browser.
				// That's why, for this particular example, the user first needs to provide the answer
				// to the browser then open the third party application. Therefore we must not kill
				// the forward on "connection refused" errors
				if opError, ok := err.(*net.OpError); ok && opError.Err.Error() == "write: connection refused" {
					continue
				}
				panic(err)
			}
		}
	})

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
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
	http.HandleFunc("/webrtc/answer", func(w http.ResponseWriter, r *http.Request) {
		// Wait for the offer to be pasted
		offer := SessionDescription{}
		bytes, err := ioutil.ReadAll(r.Body)
		err = json.Unmarshal(bytes, &offer)
		if err != nil {
			Println(err)
		}
		// Set the remote SessionDescription
		if err = peerConnection.SetRemoteDescription(offer); err != nil {
			panic(err)
		}

		// Create answer
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			panic(err)
		}

		// Sets the LocalDescription, and starts our UDP listeners
		if err = peerConnection.SetLocalDescription(answer); err != nil {
			panic(err)
		}
		bytes, err = json.Marshal(answer)
		if err != nil {
			panic(err)
		}
		w.Write(bytes)
	})
}
