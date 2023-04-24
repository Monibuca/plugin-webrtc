_[简体中文](https://github.com/Monibuca/plugin-webrtc) | English_
# WebRTC Plugin

This plugin provides the functionality to stream videos to Monibuca through a web page and to play streams from Monibuca using WebRTC technology. It follows the WHIP specification.

## Plugin URL

https://github.com/Monibuca/plugin-webrtc

## Plugin Import

```go
    import (  _ "m7s.live/plugin/webrtc/v4" )
```

## Default Configuration

```yaml
webrtc:
  iceservers: []
  publicip: [] # can be an array or a single string (automatically converted to an array)
  port: tcp:9000 # can be a range of ports like udp:8000-9000 or a single port like udp:9000
  pli: 2s # 2s
```

### ICE Server Configuration Format

```yaml
webrtc:
  iceservers:
    - urls: 
        - stun:stun.l.google.com:19302
        - turn:turn.example.org
      username: user
      credential: pass
```

### Configuration for Local Testing

If testing locally, no change in configuration is required. However, if you are accessing it remotely, then you need to configure the public IP.

## Basic Principle

The exchange of SDP messages between the browser and Monibuca takes place and RTP packets are read or sent to stream videos.

## API

### Play address
`/webrtc/play/[streamPath]`

Body: `SDP`

Content-Type: `application/sdp`

Response Body: `SDP`

### Push address

`/webrtc/push/[streamPath]`

Body: `SDP`

Content-Type: `application/sdp`

Response Body: `SDP`

### Push Test Page

`/webrtc/test/publish`

## WHIP

WebRTC-HTTP ingestion protocol
A specification for the exchange of SDP messages between WebRTC clients.

[WHIP ietf](https://datatracker.ietf.org/doc/html/draft-ietf-wish-whip-02)