package web

import (
	"bytes"
	"context"
	"d-channel/ipfsnode"
	"d-channel/secret"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"sync"
	"time"

	"d-channel/localstore"

	"filippo.io/age"
	"github.com/gin-gonic/gin"
	files "github.com/ipfs/go-ipfs-files"
	icore "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/ipfs/kubo/core"
	"github.com/libp2p/go-libp2p/core/crypto"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

const indexFile = "post.json"
const metaFile = "meta.json"

var IpfsAPI icore.CoreAPI
var IpfsNode *core.IpfsNode

func Start(addr string) error {

	IpfsAPI = ipfsnode.IpfsAPI
	IpfsNode = ipfsnode.IpfsNode

	router := gin.Default()

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
	router.GET("/getpubkey", getPubkeyHandler)             //获得自己的pubkey和peerid
	router.GET("/listipnskey", listIpnsKeyHandler)         //列出自己的ipnskey
	router.GET("/listenfolloweds", listenFollowedsHandler) //监听跟进的ipns，返回stream message

	router.POST("/publish", publishHandler)             // 发布
	router.POST("/newipnskey", newIpnsKeyHandler)       // 新建一个ipns地址
	router.POST("/reomveipnskey", removeIpnsKeyHandler) // 删除一个ipns地址
	router.POST("/newsecretkey", newSecretKeyHandler)   // 新建一个加密键（提供新旧密码，并且会替换密码）
	router.POST("/getsecretkey", getSecretKeyHandler)   // 获得加密键（需要密码，如果没有会创建）
	router.POST("/follow", followHandler)               // 添加关注
	router.POST("/addpeer", addPeertHandler)            // 添加对等好友
	router.POST("/unfollow", unFollowHandler)           // 删除关注
	router.POST("/removepeer", removePeertHandler)      // 删除好友

	router.POST("/listenp2p", listenP2PHandler) //开启监听p2p，返回stream message
	router.POST("/sendp2p", sendP2PHandler)     //发送p2p消息

	router.POST("/setstream", setStreamHandler) //开启监听p2p，返回stream message
	router.POST("/newstream", newStreamHandler) //发送p2p消息

	router.POST("/pubtopic", pubTopicHandler) //pubsub 发布topic
	router.POST("/subtopic", subTopicHandler) //pubsub 订阅topic

	router.GET("/index", indexHandler)

	return router.Run(addr)
}

// 解析ipns
func ipnsHandler(c *gin.Context) {
	ns := c.Param("ns")
	fpath := c.Param("path")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fullPath, err := IpfsAPI.Name().Resolve(ctx, ns+fpath)

	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("err %s", err.Error())))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, map[string]string{"path": fullPath.String()}))
}

// 获得ipfs数据
func ipfsHandler(c *gin.Context) {
	cid := c.Param("cid")
	fpath := c.Param("path")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nd, err := IpfsAPI.Unixfs().Get(ctx, path.New(cid+fpath))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("err %s", err.Error())))
		return
	}
	defer func() {
		nd.Close()
		_ = IpfsAPI.Pin().Add(ctx, path.New(cid+fpath))
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
	ipnskey, err := getIpnsKey(postparams.NSname)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("get  key err: %s", err.Error())))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if !postparams.Init {
		next, err := IpfsAPI.Name().Resolve(ctx, ipnskey.Path().String())
		if err != nil {
			c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("resolve err: %s", err.Error())))
			return
		}
		postparams.Next = next.String()
	}

	/// --- 3. 获得组织加密公钥
	//如果postparams.To 有值，那么加上自己的 secretkey ,否则自己是看不到自己发的消息的
	tos := []age.Recipient{}
	if len(postparams.To) > 0 {
		postparams.To = append(postparams.To, secret.Get().Recipient.(*age.X25519Recipient).String())
		for _, to := range postparams.To {
			t, err := age.ParseX25519Recipient(to)
			if err != nil {
				continue
			}
			tos = append(tos, t)
		}
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
	metaJson, _ := json.Marshal(postparams.meta)
	postMap[metaFile] = files.NewBytesFile(metaJson)

	/// --- 7. 将整个 postMap（包含post.json和所有附件）， 添加到IPFS网络,获得cid
	cid, err := IpfsAPI.Unixfs().Add(ctx, files.NewMapDirectory(postMap))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("add err: %s", err.Error())))
		return
	}

	/// --- 8. 发布新的cid到self IPNS
	nsEntry, err := IpfsAPI.Name().Publish(ctx, cid, options.Name.Key(ipnskey.Name()))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("publish err: %s", err.Error())))
		return
	}

	log.Println("name:", nsEntry.Name(), "value:", nsEntry.Value().String())
	//返回结果
	c.JSON(http.StatusOK, ResponseJsonFormat(1, map[string]string{
		"name":  nsEntry.Name(),
		"value": nsEntry.Value().String(),
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key, err := IpfsAPI.Key().Generate(ctx, nsname)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, key.Path().String()))
}

// 移除ipfs key
func removeIpnsKeyHandler(c *gin.Context) {
	nsname := c.DefaultPostForm("nsname", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key, err := IpfsAPI.Key().Remove(ctx, nsname)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, key.Path().String()))
}

// 列出所有IPFS key，也就是列出所有IPNS
func listIpnsKeyHandler(c *gin.Context) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	keys, err := IpfsAPI.Key().List(ctx)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
	}
	keysArr := [][]string{}
	for _, key := range keys {
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

func getPubkeyHandler(c *gin.Context) {

	b, err := crypto.MarshalPublicKey(IpfsNode.PrivateKey.GetPublic())
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1,
		map[string]string{
			"peerid": IpfsNode.Identity.String(),
			"pubkey": base64.RawURLEncoding.EncodeToString(b),
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

	err := localstore.AddFollow(name, ns)
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
	peerPubKey := c.DefaultPostForm("peerpubkey", "")
	peerID := c.DefaultPostForm("peerid", "")
	if name == "" && recipient == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "null name or recipient"))
		return
	}

	pidbyte, err := base64.RawURLEncoding.DecodeString(peerPubKey)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "b64:"+err.Error()))
		return
	}

	pk, err := crypto.UnmarshalPublicKey(pidbyte)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "unmarshal:"+err.Error()))
		return
	}

	pid, err := peer.IDFromPublicKey(pk)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "idfrompk:"+err.Error()))
		return
	}

	if pid.String() != peerID {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "peerid not matches"))
		return
	}

	err = localstore.AddPeer(name, recipient, peerPubKey, peerID)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func(ctx context.Context) {
		defer close(chanStream)
		for {
			select {
			case <-time.After(5 * time.Second):
				follows, _ := localstore.GetFollows(0, -1)
				for _, a := range follows {
					path, err := IpfsAPI.Name().Resolve(c, a.NS)
					if err != nil {
						continue
					}
					if a.Latest != path.String() {
						a.Latest = path.String()
						a.Save()

						u, _ := json.Marshal(a)
						chanStream <- string(u)
					}
				}

			case <-ctx.Done():
				return
			}

		}
	}(ctx)

	c.Stream(func(w io.Writer) bool {
		if msg, ok := <-chanStream; ok {
			c.SSEvent("message", msg)
			return true
		}
		return false
	})

}

// 监听p2p消息处理
func listenP2PHandler(c *gin.Context) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var readchan = make(chan []byte, 10)

	go func(readchan chan []byte) {
		defer close(readchan)
		if err := ipfsnode.ListenLocal(
			ctx,
			readchan,
			c.DefaultPostForm("port", "8090"),
		); err != nil {
			log.Println(err.Error())
		}

	}(readchan)

	var err error
	c.Stream(func(w io.Writer) bool {
		if msg, ok := <-readchan; ok {
			log.Println("read from chan", string(msg))
			err = localstore.WriteMessage(string(msg))
			log.Println("write to localstore", string(msg), err)
			if err != nil {
				c.SSEvent("message", err.Error())
				return false
			}
			c.SSEvent("message", msg)
			log.Println("send to SSEvent", string(msg))
			return true
		}
		return false
	})

	c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))

}

// 发送p2p数据处理
func sendP2PHandler(c *gin.Context) {
	// 封装发送socket

	err := ipfsnode.SendMessage(
		c.DefaultPostForm("peerid", ""),
		c.DefaultPostForm("body", ""),
		c.DefaultPostForm("port", "8091"),
	)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, "ok"))
}

func setStreamHandler(c *gin.Context) {

	var readchan = make(chan string, 10)

	ipfsnode.SetStreamHandler(readchan)

	var err error
	c.Stream(func(w io.Writer) bool {
		if msg, ok := <-readchan; ok {
			log.Println("read from chan", msg)
			err = localstore.WriteMessage(msg)
			log.Println("write to localstore", msg, err)
			if err != nil {
				c.SSEvent("message", err.Error())
				return false
			}
			c.SSEvent("message", msg)
			log.Println("send to SSEvent", msg)
			return true
		}
		return false
	})

	c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))

}

// 发送stream数据处理
func newStreamHandler(c *gin.Context) {
	// 封装发送socket

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := ipfsnode.NewStream(ctx,
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	topic := c.DefaultPostForm("topic", "")
	if topic == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "nedd topic"))
		return
	}

	sub, err := IpfsAPI.PubSub().Subscribe(ctx, topic)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	defer sub.Close()

	c.Stream(func(w io.Writer) bool {
		var msg icore.PubSubMessage
		if msg, err = sub.Next(ctx); err == nil {

			if err != nil {
				c.SSEvent("message", err.Error())
				return false
			}

			msgJsonByte, _ := json.Marshal(map[string]string{
				"seq":  string(msg.Seq()),
				"form": msg.From().String(),
				"data": string(msg.Data()),
			})

			c.SSEvent("message", string(msgJsonByte))

			return true
		}
		return false
	})

	c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))

}

// 发布Topic消息
func pubTopicHandler(c *gin.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	topic := c.DefaultPostForm("topic", "")
	msg := c.DefaultPostForm("message", "")
	if topic == "" || msg == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "nedd topic and message"))
		return
	}

	err := IpfsAPI.PubSub().Publish(ctx, topic, []byte(msg))
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
func getIpnsKey(name string) (icore.Key, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	To   []string `json:"to" form:"to"`
	Next string   `json:"next" form:"next"`
}

// 列表查询常规参数
type listParams struct {
	Limit int `form:"limit,default=10"`
	Skip  int `form:"skip,default=0"`
}

// pubsub data 对象
type subdata struct {
	Code string `json:"code"`
	Body string `json:"body"`
}
