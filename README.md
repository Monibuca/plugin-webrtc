# webrtc 插件

提供通过网页发布视频到monibuca，以及从monibuca拉流通过webrtc进行播放的功能

# 基本原理

通过浏览器和monibuca交换sdp信息，然后读取rtp包或者发送rtp的方式进行

# 界面操作

## publish
在后台管理界面，会自动获取摄像头，在streamPath中填入需要发布流的名称，点击publish开始发布

## play
在后台管理界面，点击上方play tab页此时会显示当前monibuca中所有正在发布的流，点击其中需要播放的流，会出现按钮，点击Play按钮即可弹出播放器界面并进行播放。
