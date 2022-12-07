package web

import (
	"bytes"
	"context"
	"d-channel/ipfsnode"
	"d-channel/secret"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"d-channel/localstore"

	"filippo.io/age"
	"github.com/gen2brain/beeep"
	"github.com/gin-gonic/gin"
	files "github.com/ipfs/go-ipfs-files"
	icore "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/ipfs/kubo/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"gorm.io/gorm"
)

const indexFile = "post.json"
const metaFile = "meta.json"

var IpfsAPI icore.CoreAPI
var IpfsNode *core.IpfsNode

func Start(addr string) error {

	IpfsAPI = ipfsnode.IpfsAPI
	IpfsNode = ipfsnode.IpfsNode

	router := gin.Default()
	router.SetTrustedProxies([]string{"127.0.0.1", "localhost"})

	//设置静态文件
	router.Static("/asset", "./asset")
	//设置模板文件地址
	router.LoadHTMLGlob("view/*")

	router.GET("/ipns/:ns/*path", ipnsHandler)             //
	router.GET("/ipfs/:cid/*path", ipfsHandler)            //
	router.GET("/getrecipient", getRecipientHandler)       //得到自己的加密key
	router.GET("/getfollows", getFollowsHandler)           //获得所有关注的ipns
	router.GET("/getpeers", getPeersHandler)               //从数据库中获得所有对等好友
	router.GET("/getmessages", getMessagesHandler)         //从数据库中获得p2p消息
	router.GET("/getid", getIDHandler)                     //获得自己peerid
	router.GET("/listipnskey", listIpnsKeyHandler)         //列出自己的ipnskey
	router.GET("/listenfolloweds", listenFollowedsHandler) //监听跟进的ipns，返回stream message

	router.POST("/publish", publishHandler)             // 发布
	router.POST("/newipnskey", newIpnsKeyHandler)       // 新建一个ipns地址
	router.POST("/removeipnskey", removeIpnsKeyHandler) // 删除一个ipns地址
	router.POST("/newsecretkey", newSecretKeyHandler)   // 新建一个加密键（提供新旧密码，并且会替换密码）
	router.POST("/getsecretkey", getSecretKeyHandler)   // 获得加密键（需要密码，如果没有会创建）
	router.POST("/follow", followHandler)               // 添加关注
	router.POST("/addpeer", addPeertHandler)            // 添加对等好友
	router.POST("/unfollow", unFollowHandler)           // 删除关注
	router.POST("/removepeer", removePeertHandler)      // 删除好友

	router.GET("/setstream", setStreamHandler)  //开启监听p2p，返回stream message
	router.POST("/newstream", newStreamHandler) //发送p2p消息

	router.POST("/pubtopic", pubTopicHandler) //pubsub 发布topic
	router.GET("/subtopic", subTopicHandler)  //pubsub 订阅topic

	router.GET("/index", indexHandler)
	router.GET("/", indexHandler)

	router.GET("/test", testHandler)

	return router.Run(addr)
}

func testHandler(c *gin.Context) {

	connInfos, err := IpfsAPI.Swarm().Peers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
	}

	result := []gin.H{}
	for _, cf := range connInfos {

		latency, latencyErr := cf.Latency()
		protoIDs, streamsErr := cf.Streams()
		result = append(result, gin.H{
			"id":         cf.ID(),
			"address":    cf.Address(),
			"direction":  cf.Direction(),
			"latency":    latency,
			"latencyErr": latencyErr,
			"protoIDs":   protoIDs,
			"streamsErr": streamsErr,
		})

	}

	addr1, err := peer.AddrInfoFromString("/ip4/1.14.102.100/tcp/4001/p2p/12D3KooWBBbdgzJBLUUFhMpA9JucE932wJNt2d6QZrGgSmPvTtPZ")
	if err != nil {
		log.Println("addr1", err)
	}
	err = IpfsAPI.Swarm().Connect(c.Request.Context(), *addr1)
	if err != nil {
		log.Println("conn1", err)
	}

	addr2, err := peer.AddrInfoFromString("/ip4/1.14.102.100/udp/4001/quic/p2p/12D3KooWBBbdgzJBLUUFhMpA9JucE932wJNt2d6QZrGgSmPvTtPZ")
	if err != nil {
		log.Println("addr2", err)
	}
	err = IpfsAPI.Swarm().Connect(c.Request.Context(), *addr2)
	if err != nil {
		log.Println("conn2", err)
	}

	c.JSON(http.StatusOK, result)

}

// 解析ipns
func ipnsHandler(c *gin.Context) {
	nsValue := c.Param("ns")
	fpath := c.Param("path")

	if !strings.HasPrefix(nsValue, "/ipns/") {
		nsValue = "/ipns/" + nsValue
	}

	ns, err := localstore.GetOneFollow(nsValue)
	if err != nil {
		log.Println("err", err)
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("need add follow %s", err.Error())))
		return
	}

	if ns.Latest == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "wait moment"))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, map[string]string{"path": ns.Latest + fpath}))
}

// 获得ipfs数据
func ipfsHandler(c *gin.Context) {
	cid := c.Param("cid")
	fpath := c.Param("path")

	nd, err := IpfsAPI.Unixfs().Get(c.Request.Context(), path.New(cid+fpath))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("err %s", err.Error())))
		return
	}
	defer func() {
		nd.Close()
		if c.DefaultQuery("pin", "yes") != "no" {
			log.Println("pin:", path.New(cid+fpath).String())
			err = IpfsAPI.Pin().Add(context.Background(), path.New(cid+fpath))
			if err != nil {
				log.Println("pin err:", err)
			}
		}
	}()

	fs := []string{}
	files.Walk(nd, func(fpath string, nd files.Node) error {
		fs = append(fs, fpath)
		return nil
	})

	//如果是单文件，就以文件处理，如果多文件，就认为是目录，列出所有文件
	if len(fs) != 1 {
		c.JSON(http.StatusOK, fs)
		return
	}

	f := files.ToFile(nd)
	if f == nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "not a file"))
		return
	}

	//在这里解密
	//如果不是密闻，Decrypt会自动判断，返回原始数据
	o := bytes.NewBuffer([]byte{})
	err = secret.Decrypt(secret.Get().Identities, f, o)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("decrypt err %s", err.Error())))
		return
	}
	data := o.Bytes()

	c.Data(http.StatusOK, http.DetectContentType(data), data)

}

var publishLock sync.Mutex

func publishHandler(c *gin.Context) {
	// 锁，publish 同时只能1次
	publishLock.Lock()
	defer publishLock.Unlock()

	/// --- 0. 判断是否提取密钥文件
	if secret.Get() == nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "getsecretkey first"))
		return
	}

	/// --- 1. 定义一个files.Nodes map,用于上传到IPFS网络
	postMap := map[string]files.Node{}
	//从请求中提取出请求内容
	var postparams postParams
	err := c.ShouldBind(&postparams)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("bind err: %s", err.Error())))
		return
	}

	/// --- 2. 解析出self发布的最新的cid，并写入到post中的next字段
	ipnskey, err := getIpnsKey(c.Request.Context(), postparams.NSname)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("get  key err: %s", err.Error())))
		return
	}

	thisNS, err := localstore.GetOneFollow(ipnskey.Path().String())
	if err != nil && thisNS.IsSelf {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("resolve err: %v %v", err, thisNS.IsSelf)))
		return
	}
	postparams.Next = thisNS.Latest

	/// --- 3. 获得组织加密公钥
	//如果postparams.To 有值，那么加上自己的 secretkey ,否则自己是看不到自己发的消息的
	tos := []age.Recipient{}
	if len(postparams.To) > 0 {
		postparams.To = append(postparams.To, secret.Get().Recipient.(*age.X25519Recipient).String())
		formatedTo := []string{}
		for _, to := range postparams.To {
			t, err := age.ParseX25519Recipient(to)
			if err != nil {
				continue
			}
			tos = append(tos, t)
			formatedTo = append(formatedTo, to)
		}
		postparams.To = formatedTo // 重新赋值整理后的to列表，前面的循环去掉了age.ParseX25519Recipient失败的键
	}

	/// --- 4. 从请求内容中，提取出需要上传的文件，并写入到 postMap, 修改post中附件文件路径为文件名
	err = uploadFiles(&postparams, postMap, tos)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	/// --- 5. 对post对象加密（如果不需要加密则保存原始数据）
	data, _ := json.Marshal(postparams.post)
	if len(tos) > 0 {
		f := bytes.NewBuffer(data)
		o := bytes.NewBuffer([]byte{})
		err = secret.Encrypt(tos, f, o)
		if err != nil {
			c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("encrypt post err: %s", err.Error())))
			return
		}
		data = o.Bytes()
	}
	postMap[indexFile] = files.NewBytesFile(data)

	/// --- 6. 将meta对象，重新序列化为json，并作为meta.json文件保存
	postparams.meta.CreatedAt = time.Now()
	metaJson, _ := json.Marshal(postparams.meta)
	postMap[metaFile] = files.NewBytesFile(metaJson)

	/// --- 7. 将整个 postMap（包含post.json和所有附件）， 添加到IPFS网络,获得cid
	cid, err := IpfsAPI.Unixfs().Add(c.Request.Context(), files.NewMapDirectory(postMap))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("add err: %s", err.Error())))
		return
	}

	log.Println("----Publishing---")
	/// --- 8. 发布新的cid到self IPNS
	go func() {
		nsEntry, err := IpfsAPI.Name().Publish(context.Background(), cid,
			options.Name.Key(ipnskey.Name()),
			// options.Name.ValidTime(time.Hour*24),
		)

		if err != nil {
			log.Println("publish err", err)
			return
		}

		log.Println("publish success", nsEntry)

	}()

	thisNS.Latest = cid.String()
	err = thisNS.Save()
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("save err: %s", err.Error())))
		return
	}

	// log.Println("name:", nsEntry.Name(), "value:", nsEntry.Value().String())
	//返回结果
	c.JSON(http.StatusOK, ResponseJsonFormat(1, map[string]string{
		"name":  ipnskey.Name(),
		"value": cid.String(),
	}))
}

// 从请求内容中，提取出需要上传的文件，并写入到 postMap, 修改post中附件文件路径为文件名
func uploadFiles(postparams *postParams, postMap map[string]files.Node, tos []age.Recipient) error {
	//迭代所有附件
	//文件名不能为post,也不能重复
	// 如果有to，说明需要加密
	// 使用to里面的公钥加密文件
	// 获得文件加密后的数据（或者不需要加密的原始数据），并且保存到postMap
	for i, u := range postparams.Uploads {
		log.Println(i, u.Filename)
		if _, ok := postMap[u.Filename]; u.Filename == indexFile || ok {
			return fmt.Errorf("filename err: %s %t %t", u.Filename, u.Filename == indexFile, ok)
		}

		postparams.Attachments = append(postparams.Attachments, u.Filename)

		f, err := u.Open()
		if err != nil {
			return fmt.Errorf("open file err: %s", err.Error())
		}

		var data []byte

		if len(postparams.To) > 0 {

			o := bytes.NewBuffer([]byte{})
			err = secret.Encrypt(tos, f, o)
			if err != nil {
				return fmt.Errorf("encrypt file err: %s", err.Error())
			}

			data = o.Bytes()
		} else {
			data, err = io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("read file err: %s", err.Error())
			}

		}

		postMap[u.Filename] = files.NewBytesFile(data)

		f.Close()
	}
	return nil
}

// 新增IPFS key，也就是新增一个IPNS
func newIpnsKeyHandler(c *gin.Context) {
	// options.NamePublishOption
	nsname := c.DefaultPostForm("nsname", "")

	key, err := IpfsAPI.Key().Generate(c.Request.Context(), nsname)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	err = localstore.AddFollow(nsname, key.Path().String(), true)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, key.Path().String()))
}

// 移除ipfs key
func removeIpnsKeyHandler(c *gin.Context) {
	nsname := c.DefaultPostForm("nsname", "")

	key, err := IpfsAPI.Key().Remove(c.Request.Context(), nsname)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	err = localstore.DelSelfNS(nsname)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, key.Path().String()))
}

// 列出所有IPFS key，也就是列出所有IPNS
func listIpnsKeyHandler(c *gin.Context) {

	keys, err := IpfsAPI.Key().List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
	}
	keysArr := [][]string{}
	for _, key := range keys {

		_, err := localstore.GetOneFollow(key.Path().String())
		if err == gorm.ErrRecordNotFound {
			err = localstore.AddFollow(key.Name(), key.Path().String(), true)
			if err != nil {
				continue
			}
		}

		keysArr = append(keysArr, []string{key.Name(), key.Path().String()})
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, keysArr))
}

// 使用一个新的密钥，会保留原来的私钥，新增一对公私钥，返回新的公钥
func newSecretKeyHandler(c *gin.Context) {
	var err error
	_, err = secret.NewSecretKey(c.DefaultPostForm("oldpassword", ""), c.DefaultPostForm("password", ""))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, secret.Get().Recipient.(*age.X25519Recipient).String()))
}

func getSecretKeyHandler(c *gin.Context) {
	var err error
	_, err = secret.GenSecretKey(c.DefaultPostForm("password", ""))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, secret.Get().Recipient.(*age.X25519Recipient).String()))
}

// 获得用于加密的公钥
func getRecipientHandler(c *gin.Context) {
	if secret.Get() == nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "getsecretkey first"))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, secret.Get().Recipient.(*age.X25519Recipient).String()))
}

// 获得本地存储
func getFollowsHandler(c *gin.Context) {
	lp := listParams{}
	err := c.ShouldBind(&lp)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	follows, err := localstore.GetFollows(lp.Skip, lp.Limit)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, follows))
}

// 获得本地存储
func getPeersHandler(c *gin.Context) {
	lp := listParams{}
	err := c.ShouldBind(&lp)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	peers, err := localstore.GetPeers(lp.Skip, lp.Limit)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	//尝试连接一遍其他节点。
	go func(ps []localstore.Peer) {
		for _, p := range ps {

			pid, err := peer.Decode(p.PeerID)
			if err != nil {
				return
			}
			pInfo, err := IpfsAPI.Dht().FindPeer(context.Background(), pid)
			if err != nil {
				return
			}
			IpfsAPI.Swarm().Connect(context.Background(), pInfo)
		}
	}(peers)

	c.JSON(http.StatusOK, ResponseJsonFormat(1, peers))
}

// 获得本地存储
func getMessagesHandler(c *gin.Context) {
	lp := listParams{}
	err := c.ShouldBind(&lp)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	messages, err := localstore.GetMessages(lp.Skip, lp.Limit)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, messages))
}

func getIDHandler(c *gin.Context) {

	c.JSON(http.StatusOK, ResponseJsonFormat(1,
		map[string]string{
			"peerid": IpfsNode.Identity.String(),
		}))
}

// 订阅其他人的IPNS name
func followHandler(c *gin.Context) {
	name := c.DefaultPostForm("name", "")
	ns := c.DefaultPostForm("ns", "")
	if ns == "" && name == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "null ns or name"))
		return
	}

	err := localstore.AddFollow(name, ns, false)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, "ok"))
}

func unFollowHandler(c *gin.Context) {
	err := localstore.UnFollow(c.DefaultPostForm("id", "0"))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, "ok"))
}

// 添加Peer
func addPeertHandler(c *gin.Context) {
	name := c.DefaultPostForm("name", "")
	recipient := c.DefaultPostForm("recipient", "")
	peerID := c.DefaultPostForm("peerid", "")
	if name == "" && recipient == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "null name or recipient"))
		return
	}

	err := localstore.AddPeer(name, recipient, peerID)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, "ok"))

}

func removePeertHandler(c *gin.Context) {
	err := localstore.RemovePeer(c.DefaultPostForm("id", "0"))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, "ok"))
}

func listenFollowedsHandler(c *gin.Context) {

	chanStream := make(chan string, 1)

	go func(chanStream chan string) {
		defer close(chanStream)
		for {
			select {
			case <-time.After(30 * time.Second):
				follows, _ := localstore.GetFollows(0, -1)
				for _, a := range follows {
					path, err := IpfsAPI.Name().Resolve(c.Request.Context(), a.NS,
						options.Name.Cache(true),
					)

					if err != nil {
						log.Println("resolve follow err", err)
						continue
					}

					log.Println("get follow path", a.Name, a.NS)

					if a.Latest != path.String() {

						//解析出来后，先pin 上
						go func() {
							err = IpfsAPI.Pin().Add(c.Request.Context(), path)
							if err != nil {
								log.Println("resovle pin err:", err)
								return
							}
							log.Println("get new path and pin", a.Latest, path)
						}()

						a.Latest = path.String()
						err := a.Save()
						if err != nil {
							log.Println("save follow err", err)
							continue
						}
						log.Println("follow", a)

						u, _ := json.Marshal(a)
						chanStream <- string(u)
						_ = beeep.Notify("channel update", a.Name, "./asset/favicon.ico")

					}
				}

			case <-c.Request.Context().Done():
				log.Println("c request context Done")
				return

			}

		}
	}(chanStream)

	c.Stream(func(w io.Writer) bool {
		if msg, ok := <-chanStream; ok {
			c.SSEvent("message", msg)
			return true
		}
		return false
	})

}

func setStreamHandler(c *gin.Context) {

	var readchan = make(chan string, 10)

	ipfsnode.SetStreamHandler(readchan)
	// readchan <- "started"
	defer func() {
		log.Println("[stream handler over]")
		ipfsnode.RemoveStreamHandler()
		close(readchan)
	}()

	var err error
	c.Stream(func(w io.Writer) bool {

		select {
		case msg := <-readchan:
			if err = localstore.WriteMessage(msg); err != nil {
				c.SSEvent("message", err.Error())
				return false
			}
			c.SSEvent("message", msg)

			msgbody := map[string]string{}

			if err = json.Unmarshal([]byte(msg), &msgbody); err != nil {
				msgbody["message"] = err.Error()
			}

			_ = beeep.Notify("message", msgbody["message"], "./asset/favicon.ico")
			return true
		case <-c.Request.Context().Done():
			return false
		}

	})

}

// 发送stream数据处理
func newStreamHandler(c *gin.Context) {
	// 封装发送socket

	err := ipfsnode.NewStream(c.Request.Context(),
		c.DefaultPostForm("peerid", ""),
		c.DefaultPostForm("body", ""),
	)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, "ok"))
}

// 订阅topic
func subTopicHandler(c *gin.Context) {

	topic := c.DefaultQuery("topic", "")
	if topic == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "need topic"))
		return
	}

	sub, err := IpfsAPI.PubSub().Subscribe(c.Request.Context(), topic)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	defer sub.Close()

	c.Stream(func(w io.Writer) bool {
		var msg icore.PubSubMessage
		if msg, err = sub.Next(c.Request.Context()); err == nil {

			if err != nil {
				c.SSEvent("message", err.Error())
				return false
			}

			msgJsonByte, _ := json.Marshal(map[string]string{
				"seq":     string(msg.Seq()),
				"form":    msg.From().String(),
				"message": string(msg.Data()),
			})

			c.SSEvent("message", string(msgJsonByte))
			_ = beeep.Notify("topic", string(msg.Data()), "./asset/favicon.ico")

			return true
		}
		return false
	})

}

// 发布Topic消息
func pubTopicHandler(c *gin.Context) {
	topic := c.DefaultPostForm("topic", "")
	msg := c.DefaultPostForm("message", "")
	if topic == "" || msg == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "nedd topic and message"))
		return
	}

	err := IpfsAPI.PubSub().Publish(c.Request.Context(), topic, []byte(msg))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, "ok"))

}

// 首页处理函数
func indexHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index", gin.H{})
}

// 格式化返回的JSON，补充将Data的类型，方便前端判断
func ResponseJsonFormat(code int8, data interface{}) responseJson {
	return responseJson{
		Code: code,
		Data: data,
		Type: fmt.Sprintf("%T", data),
	}
}

// 获得IPNS的key对象，如果传递的名称没有对应的key，默认返回self
func getIpnsKey(ctx context.Context, name string) (icore.Key, error) {

	ipnsKeys, err := IpfsAPI.Key().List(ctx)
	if err != nil {
		return nil, err
	}

	for _, k := range ipnsKeys {
		if k.Name() == name {
			return k, nil
		}
	}

	return IpfsAPI.Key().Self(ctx)
}

// json返回值对象
type responseJson struct {
	Code int8        `json:"code"`
	Data interface{} `json:"data"`
	Type string      `json:"type"`
}

// publish的表单request表单对象
type postParams struct {
	post                            //save to post.json
	meta                            //save to meta.json
	Uploads []*multipart.FileHeader `json:"-" form:"uploads"` //upload field
	NSname  string                  `json:"-" form:"nsname"`  //which key to publish
	Init    bool                    `json:"-" form:"init,default=false"`
}

// 主内容对象
type post struct {
	Body        string   `json:"body" form:"body"`
	Type        string   `json:"type" form:"type,default=plaintext"`
	Attachments []string `json:"attachments" form:"-"`
}

// 元数据对象，这个对象中的内容将不加密
type meta struct {
	CreatedAt time.Time `json:"createdAt" form:"-"`
	To        []string  `json:"to" form:"to"`
	Next      string    `json:"next" form:"next"`
}

// 列表查询常规参数
type listParams struct {
	Limit int `form:"limit,default=10"`
	Skip  int `form:"skip,default=0"`
}
