module github.com/Monibuca/plugin-webrtc/v3

go 1.13

require (
	github.com/Monibuca/engine/v3 v3.0.0
	github.com/Monibuca/plugin-rtp v1.0.0
	github.com/Monibuca/utils/v3 v3.0.0-alpha2
	github.com/pion/rtcp v1.2.6
	github.com/pion/webrtc/v3 v3.0.4
	github.com/shirou/gopsutil v2.20.8+incompatible // indirect
)

replace github.com/Monibuca/engine/v3 v3.0.0 => ../engine

replace github.com/Monibuca/utils/v3 => ../utils
