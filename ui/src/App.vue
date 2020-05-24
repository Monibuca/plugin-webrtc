<template>
  <div class="root">
    Browser SDP
    <br />
    <textarea readonly="true" rows="10" cols="100">{{localSDP||"loading..."}}</textarea>
    <br />Golang SDP
    <br />
    <textarea rows="10" cols="100">{{remoteSDP}}</textarea>
    <br />
    <mu-text-field v-model="streamPath" label="streamPath"></mu-text-field>
    <m-button @click="startSession" v-if="localSDP">Start</m-button>
    <br />

    <br />Video
    <br />
    <video id="video1" width="160" height="120" autoplay muted></video>
    <br />Logs
    <br />
    <div id="logs"></div>
  </div>
</template>

<script>
let pc = new RTCPeerConnection({
  iceServers:[
    {
      urls:[
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
      ]
    }
  ]
});
export default {
  data() {
    return {
      localSDP: "",
      remoteSDP: "",
      streamPath:"live/rtc"
    };
  },
  methods: {
    startSession() {
      this.ajax({type: 'POST',processData:false,data: JSON.stringify(pc.localDescription),url:"/webrtc/answer?streamPath="+this.streamPath,dataType:"json"}).then(result => {
        this.remoteSDP = result.sdp;
        pc.setRemoteDescription(new RTCSessionDescription(result));
      });
    }
  },
  mounted() {
    /* eslint-env browser */
    var log = msg => {
      document.getElementById("logs").innerHTML += msg + "<br>";
    };

    navigator.mediaDevices
      .getUserMedia({ video: true, audio: true })
      .then(stream => {
        pc.addStream((document.getElementById("video1").srcObject = stream));
        pc.createOffer()
          .then(d => pc.setLocalDescription(d))
          .catch(log);
      })
      .catch(log);

    pc.oniceconnectionstatechange = e => log(pc.iceConnectionState);
    pc.onicecandidate = event => {
      if (event.candidate === null) {
        this.localSDP = pc.localDescription.sdp;
      }
    };
  }
};
</script>

<style>
.root textarea{
  background: transparent;
  color: cyan;
}
</style>