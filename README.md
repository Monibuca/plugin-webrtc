# WebRTC 插件

提供通过网页发布视频到monibuca，以及从monibuca拉流通过webrtc进行播放的功能，遵循WHIP规范

## 插件地址

https://github.com/Monibuca/plugin-webrtc

## 插件引入
```go
    import (  _ "m7s.live/plugin/webrtc/v4" )
```

## 默认配置

```yaml
webrtc:
  iceservers: []
  publicip: [] # 可以是数组也可以是字符串（内部自动转成数组）
  portmin: 0
  portmax: 0
  inviteportfixed: true  // 设备将流发送的端口，是否固定  on 发送流到多路复用端口 如9000  off 自动从 mix_port - max_port 之间的值中  选一个可以用的端口
  iceudpmux:       9000  // 接收设备端rtp流的多路复用端口
  pli: 2000000000 # 2s
```

### 本地测试无需修改配置，如果远程访问，则需要配置publicip

## 基本原理

通过浏览器和monibuca交换sdp信息，然后读取rtp包或者发送rtp的方式进行

## API

### 播放地址
`/webrtc/play/[streamPath]`

Body: `SDP`

Content-Type: `application/sdp`

Response Body: `SDP`

### 推流地址

`/webrtc/push/[streamPath]`

Body: `SDP`

Content-Type: `application/sdp`

Response Body: `SDP`
## WHIP
WebRTC-HTTP ingestion protocol
用于WebRTC交换SDP信息的规范

[WHIP ietf](https://datatracker.ietf.org/doc/html/draft-ietf-wish-whip-02)
