# d-channel

Personal information publish channel base on IPFS

## How it works

每一个IPNS KEY就是一个频道地址，也可以称之为 addr

addr 被指向到一个CID，并且在IPFS节点之间共享。

d-channel 上编写并发布内容，内容将会以以下结构的目录发布到IPFS网络上

```
channel
|- post.json
|- meta.json
|- file1
|- file2
|- ...
```

发布成功后，将addr指向到channel的cid。上一次发布的cid会被写入到 meta.json 的 next字段。通过迭代读取就可以获得全部的发布内容。

发布时，可以使用age进行加密。

## peer

对等体，包含 别名、reicipent、peerid、pubkey(ipfs节点的公钥匙)

peerid可以从pubkey中得到(peer.id.frompubkey)

peerid将用于p2p通信。

reicipent用于发布和p2p加密

## follow

监听的其他人的NS

## P2P

开启本地p2p监听，默认 proto /x/message

发送消息到指定peerid


## 一些问题

- IPFS节点离线后，无论是DHT还是CID可能都会无法使用，DHT默认会保存10小时，但是如果没有其他节点PIN CID ，CID都将是无法访问的。如何实现离线数据上传和下载？
   1. 利用区块链，优化DHT
   2. 利用远端PIN服务，优化存储 
- PUBSUB 临时的、一次性的，如果使用？
  1. pub消息，带上上一个消息的cid
  2. 同时只能订阅一个topic，类似加入直播间
- IPNS工作方式与预期不同
  1. 当节点离线，解析不了ipns


## todo

- [x] ~~ipfs访问默认pin，除非传递 ?pin=no~~
- [x] ~~ipfs v0  ID 改为v1~~   
    改用ED25519生成ID和私钥即可
- [x] ~~ipfs简短版的公钥匙~~
- [ ] 发现任何对等节点，都尝试Swarm connect
- [ ] 发布内容的时间戳问题
- [ ] 融入区块链（智能合约的开发）、文件币,区块链作为DHT升级和替代，文件比和远程PIN 作为存储服务
- [x] 添加自己的频道到数据库，并且每次发布时，从数据库提取上一个
- [x] 取消init的设置
- [ ] 取掉pubkey的设置

 
```
 API Key: 5afc0d82ad998caa6d61
 API Secret: fce34c78c75a1cc597140130271b9c030dc0d349e868b79a0afb00eef8c2d090
 JWT: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySW5mb3JtYXRpb24iOnsiaWQiOiIzZWYwYWM4ZS1kZDMxLTRlMjktYjJkYi1jNTI5NTI5MjEzMGYiLCJlbWFpbCI6InpldGFjaG93QDE2My5jb20iLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwicGluX3BvbGljeSI6eyJyZWdpb25zIjpbeyJpZCI6IkZSQTEiLCJkZXNpcmVkUmVwbGljYXRpb25Db3VudCI6MX0seyJpZCI6Ik5ZQzEiLCJkZXNpcmVkUmVwbGljYXRpb25Db3VudCI6MX1dLCJ2ZXJzaW9uIjoxfSwibWZhX2VuYWJsZWQiOmZhbHNlLCJzdGF0dXMiOiJBQ1RJVkUifSwiYXV0aGVudGljYXRpb25UeXBlIjoic2NvcGVkS2V5Iiwic2NvcGVkS2V5S2V5IjoiNWFmYzBkODJhZDk5OGNhYTZkNjEiLCJzY29wZWRLZXlTZWNyZXQiOiJmY2UzNGM3OGM3NWExY2M1OTcxNDAxMzAyNzFiOWMwMzBkYzBkMzQ5ZTg2OGI3OWEwYWZiMDBlZWY4YzJkMDkwIiwiaWF0IjoxNjY5NjIyNDI4fQ.krUZ__4JB6M1YRS-q6n3PA9xWZTSgxuwk_EObBZCqsM
 
```
