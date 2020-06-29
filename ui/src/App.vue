<template>
    <div>
        <pre v-if="$parent.titleTabActive == 1">{{localSDP}}</pre>
        <pre v-else-if="$parent.titleTabActive == 2">{{remoteSDP}}</pre>
        <div v-else>
            <mu-text-field v-model="streamPath" label="streamPath"></mu-text-field>
            <span class="blink" v-if="!localSDP || ask">Connecting</span>
            <template>
                <m-button @click="startSession('publish')">Publish</m-button>
                <m-button @click="startSession('play')">Play</m-button>
            </template>
            <m-button @click="stopSession">Stop</m-button>
            <br />
            <video ref="video1" :srcObject.prop="stream" width="640" height="480" autoplay muted></video>
        </div>
    </div>
</template>

<script>
const config = {
    iceServers: [
        // {
        //   urls:[
        //     "stun:stun.ekiga.net",
        //     "stun:stun.ideasip.com",
        //     "stun:stun.schlund.de",
        //     "stun:stun.stunprotocol.org:3478",
        //     "stun:stun.voiparound.com",
        //     "stun:stun.voipbuster.com",
        //     "stun:stun.voipstunt.com",
        //     "stun:stun.voxgratia.org",
        //     "stun:stun.services.mozilla.com",
        //     "stun:stun.xten.com",
        //     "stun:stun.softjoys.com",
        //     "stun:stunserver.org",
        //     "stun:stun.schlund.de",
        //     "stun:stun.rixtelecom.se",
        //     "stun:stun.iptel.org",
        //     "stun:stun.ideasip.com",
        //     "stun:stun.fwdnet.net",
        //     "stun:stun.ekiga.net",
        //     "stun:stun01.sipphone.com",
        //   ]
        // }
    ]
};
let pc = new RTCPeerConnection(config);
var stream = null;
var streamPath = "live/rtc";
export default {
    data() {
        return {
            localSDP: pc && pc.localDescription && pc.localDescription.sdp,
            remoteSDP: pc && pc.remoteDescription && pc.remoteDescription.sdp,
            streamPath,
            iceConnectionState: pc && pc.iceConnectionState,
            stream,
            type: "",
            ask: false
        };
    },
    methods: {
        async startSession(type) {
            this.type = type;
            this.ask = true;
            const result = await this.ajax({
                type: "POST",
                processData: false,
                data: JSON.stringify(pc.localDescription),
                url: "/webrtc/" + type + "?streamPath=" + this.streamPath,
                dataType: "json"
            });
            this.ask = false;
            if (result.errmsg) {
                this.$toast.error(result.errmsg);
            } else {
                streamPath = this.streamPath;
                this.remoteSDP = result.sdp;
                this.$parent.titleTabs = ["摄像头", "localSDP", "remoteSDP"];
                pc.setRemoteDescription(new RTCSessionDescription(result));
            }
            if (type == "publish") {
                pc.addStream(stream);
                this.localSDP = (
                    await pc.setLocalDescription(await pc.createOffer())
                ).sdp;
            } else {
                pc.ontrack = event => {
                  if(event.streams[0].id=="monibuca")
                    this.stream = stream = event.streams[0];
                };
            }
        },
        stopSession() {
            pc.close();
            pc = new RTCPeerConnection(config);
            this.remoteSDP = "";
            this.localSDP = "";
            this.type = "";
            // this.connectICE().catch(err => this.$toast.error(err.message));
        }
    },
    async mounted() {
        pc.onsignalingstatechange = e => {
            console.log(e);
        };
        pc.oniceconnectionstatechange = e => {
            this.$toast.info(pc.iceConnectionState);
            this.iceConnectionState = pc.iceConnectionState;
        };
        pc.onicecandidate = event => {
            if (event.candidate === null) {
                this.$parent.titleTabs = ["摄像头", "localSDP"];
            }
        };
        if (this.localSDP) {
            let tabs = ["摄像头"];
            if (this.localSDP) tabs.push("localSDP");
            if (this.remoteSDP) tabs.push("remoteSDP");
            this.$parent.titleTabs = tabs;
        } else {
            try {
                if (!this.stream)
                    this.stream = stream = await navigator.mediaDevices.getUserMedia(
                        { video: true, audio: true }
                    );
            } catch (err) {
                this.$toast.error(err.message);
            }
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