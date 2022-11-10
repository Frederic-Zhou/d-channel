# dbbs

利用IPFS的name发布。

## 流程

1. 发布人使用ipfs key gen 生成 1 个Key(kx)，该kx就是公告版地址
2. 发布人 创建一个目录dbbs1，并使用ipfs name publish 发布dbbs1目录
3. 发布人将自己的加密pubkey写dbbs1 目录中pk文件里
4. 发布人将kx 告知订阅人，其他人即可查阅A 发布在dbbs1目录中的内容

订阅人如果需要发布人发布私密信息，需要提供自己的pubkey给发布者，因此，双向端对端加密，需要互相知道对方的发布地址。

## 特点

数据存储完全依托于ipfs，除了用于解密的**私钥**

因为ipfs name publish可以自验证（ipns地址就是公钥hash），因此不需要单独做签名，只需要作为订阅者解密发布者内容的**私钥**

## 目录结构

```
ddbs
|-post.json
|-file1
|-file2
|-...
```


## todo 

1. ~~解密~~
2. ~~生成自己的age key~~
3. 遍历抓取内容
4. ~~删除 ipfskey~~

