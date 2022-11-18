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

对等体，包含 别名、加密key、peerid、pubkey(ipfs节点的公钥匙)

peerid可以从pubkey中得到(peer.id.frompubkey)

## follow

IPNS KEY 亦 addr

`这里需要统一名称`