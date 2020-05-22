<template>
  <div>
    Browser base64 Session Description
    <br />
    <textarea readonly="true" :value="localDescription"></textarea>
    <br />Golang base64 Session Description
    <br />
    <textarea :value="remoteSessionDescription"></textarea>
    <br />
    <button @click="startSession">Start Session</button>
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
  iceServers: [
    {
      urls: "stun:stun.l.google.com:19302"
    }
  ]
});
export default {
  data() {
    return {
      localDescription: "",
      remoteSessionDescription: ""
    };
  },
  methods: {
    startSession() {
      this.ajax.post("/webrtc/answer", this.localDescription).then(result => {
        this.remoteSessionDescription = result;
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
        this.localDescription = JSON.stringify(pc.localDescription);
      }
    };
  }
};
</script>

<style>
</style>