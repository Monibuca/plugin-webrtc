<template>
    <div>
        <div v-if="$parent.titleTabActive == 0">
            <mu-text-field v-model="streamPath" label="streamPath"></mu-text-field>
            <m-button @click="publish" v-if="!remoteSDP">Publish</m-button>
            <m-button @click="stopSession" v-else>Stop</m-button>
            <a v-if="remoteSDP" :href="remoteSDPURL" download="remoteSDP.txt">remoteSDP</a>
            <span>&nbsp;&nbsp;</span>
            <a v-if="localSDP" :href="localSDPURL" download="localSDP.txt">localSDP</a>
            <br />
            <video ref="video1" :srcObject.prop="stream" width="640" height="480" autoplay muted></video>
        </div>
        <stream-table v-else-if="$parent.titleTabActive == 1">
            <template v-slot="scope">
                <m-button @click="preview(scope)">Play</m-button>
            <template>
        </stream-table>
        <pre v-else-if="$parent.titleTabActive == 2">{{localSDP}}</pre>
        <pre v-else-if="$parent.titleTabActive == 3">{{remoteSDP}}</pre>
        <webrtc-player ref="player" v-model="previewStreamPath"></webrtc-player>
    </div>
</template>

<script>
import WebrtcPlayer from "./components/Player"
const config = { iceServers: []};
let pc = new RTCPeerConnection(config);
var stream = null
var streamPath = "live/rtc";
export default {
    components:{
        WebrtcPlayer
    },
    data() {
        return {
            localSDP: pc && pc.localDescription && pc.localDescription.sdp,
            remoteSDP: pc && pc.remoteDescription && pc.remoteDescription.sdp,
            streamPath,
            iceConnectionState: pc && pc.iceConnectionState,
            stream,
            previewStreamPath:false,
            localSDPURL:"",
            remoteSDPURL:""
        };
    },
    methods: {
        async publish() {
            pc.addStream(stream);
            await pc.setLocalDescription(await pc.createOffer());
            this.localSDP = pc.localDescription.sdp;
            this.localSDPURL = URL.createObjectURL(new Blob([ this.localSDP ],{type:'text/plain'}))
            const result = await this.ajax({
                type: "POST",
                processData: false,
                data: JSON.stringify(pc.localDescription),
                url: "/webrtc/publish?streamPath=" + this.streamPath,
                dataType: "json"
            });
            if (result!="success") {
                this.$toast.error(result.errmsg||result);
                return;
            } else {
                streamPath = this.streamPath;
            }
            this.remoteSDP = result.sdp;
            this.remoteSDPURL = URL.createObjectURL(new Blob([ this.remoteSDP ],{type:'text/plain'}))
            pc.setRemoteDescription(new RTCSessionDescription(result));
        },
        stopSession() {
            pc.close();
            pc = new RTCPeerConnection(config);
            this.remoteSDP = "";
            this.localSDP = "";
            // this.connectICE().catch(err => this.$toast.error(err.message));
        },
        preview({row}) {
            this.previewStreamPath = true
             this.$nextTick(() =>this.$refs.player.play(row.StreamPath));
        },
    },
    async mounted() {
        pc.onsignalingstatechange = e => {
            console.log(e);
        };
        pc.oniceconnectionstatechange = e => {
            this.$toast.info(pc.iceConnectionState);
            this.iceConnectionState = pc.iceConnectionState;
        };
        pc.onicecandidate = event => {};
        this.$parent.titleTabs = ["publish","play"];
        try {
            if (!this.stream)
                this.stream = stream = await navigator.mediaDevices.getUserMedia(
                    { video: true, audio: true }
                );
        } catch (err) {
            this.$toast.error(err.message);
        }
    }
};
</script>

<style scoped>
@keyframes blink {
    0% {
        opacity: 0.2;
    }
    50% {
        opacity: 1;
    }
    100% {
        opacity: 0.2;
    }
}
.blink {
    animation: blink 1s infinite;
}
</style>