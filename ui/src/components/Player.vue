<template>
  <Modal
    v-bind="$attrs"
    draggable
    v-on="$listeners"
    :title="streamPath"
    @on-ok="onClosePreview"
    @on-cancel="onClosePreview"
  >
    <video ref="webrtc" :srcObject.prop="stream" width="488" height="275" autoplay muted controls></video>
    <div slot="footer">
      <mu-badge v-if="remoteSDP">
        <a slot="content" :href="remoteSDPURL" download="remoteSDP.txt">remoteSDP</a>
      </mu-badge>
      <mu-badge v-if="localSDP">
        <a slot="content" :href="localSDPURL" download="localSDP.txt">localSDP</a>
      </mu-badge>
    </div>
  </Modal>
</template>
<script>
let pc = null;
export default {
  data() {
    return {
      iceConnectionState: pc && pc.iceConnectionState,
      stream: null,
      localSDP: "",
      remoteSDP: "",
      remoteSDPURL: "",
      localSDPURL: "",
      streamPath: ""
    };
  },

    methods: {
        async play(streamPath) {
            pc = new RTCPeerConnection();
            pc.addTransceiver('video',{
              direction:'recvonly'
            })
            this.streamPath = streamPath;
            pc.onsignalingstatechange = e => {
                //console.log(e);
            };
            pc.oniceconnectionstatechange = e => {
                this.$toast.info(pc.iceConnectionState);
                this.iceConnectionState = pc.iceConnectionState;
            };
            pc.onicecandidate = event => {
                console.log(event)
            };
            pc.ontrack = event => {
               // console.log(event);
                if (event.track.kind == "video")
                    this.stream = event.streams[0];
            };
            await pc.setLocalDescription(await pc.createOffer());
            this.localSDP = pc.localDescription.sdp;
            this.localSDPURL = URL.createObjectURL(
                new Blob([this.localSDP], { type: "text/plain" })
            );
            const result = await this.ajax({
                type: "POST",
                processData: false,
                data: JSON.stringify(pc.localDescription.toJSON()),
                url: "/webrtc/play?streamPath=" + this.streamPath,
                dataType: "json"
            });
            if (result.errmsg) {
                this.$toast.error(result.errmsg);
                return;
            } else {
                this.remoteSDP = result.sdp;
                this.remoteSDPURL = URL.createObjectURL(new Blob([this.remoteSDP], { type: "text/plain" }));
            }
            await pc.setRemoteDescription(new RTCSessionDescription(result));
        },
        onClosePreview() {
            pc.close();
        }
    }
};
</script>