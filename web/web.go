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
	"sync"
	"time"

	"d-channel/localstore"

	"filippo.io/age"
	"github.com/gin-gonic/gin"
	files "github.com/ipfs/go-ipfs-files"
	icore "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
)

var IpfsAPI = ipfsnode.IpfsAPI
var IpfsNode = ipfsnode.IpfsNode
var SKeys = secret.SeKeys

const indexFile = "post.json"
const metaFile = "meta.json"

func Start(addr string) error {

	router := gin.Default()

	//设置静态文件
	router.Static("/asset", "./asset")
	//设置模板文件地址
	router.LoadHTMLGlob("view/*")

	router.GET("/ipns/:name/*path", ipnsHandler)                 //
	router.GET("/ipfs/:cid/*path", ipfsHandler)                  //
	router.GET("/getsecretrecipient", getSecretRecipientHandler) //
	router.GET("/getfollows", getFollowsHandler)                 //
	router.GET("/getpeers", getPeersHandler)                     //
	router.GET("/getmessages", getMessagesHandler)               //
	router.GET("/listipfskey", listIpfsKeyHandler)               //

	router.POST("/publish", publishHandler)       //
	router.POST("/newipfskey", newIpfsKeyHandler) //
	router.POST("/reomveipfskey", removeIpfsKeyHandler)
	router.POST("/newsecretkey", newSecretKeyHandler) //
	router.POST("/getsecretkey", getSecretKeyHandler) //
	router.POST("/follow", followHandler)             //
	router.POST("/addrecipient", addRecipientHandler) //
	router.GET("/listenfolloweds", listenFollowedsHandler)

	router.POST("/listenp2p", listenP2PHandler)
	router.POST("/sendp2p", sendP2PHandler)

	router.GET("/index", indexHandler)

	return router.Run(addr)
}

// 解析ipns
func ipnsHandler(c *gin.Context) {
	name := c.Param("name")
	fpath := c.Param("path")

	fullPath, err := IpfsAPI.Name().Resolve(context.Background(), name+fpath)

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

	nd, err := IpfsAPI.Unixfs().Get(context.Background(), path.New(cid+fpath))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("err %s", err.Error())))
		return
	}
	defer func() {
		nd.Close()
		_ = IpfsAPI.Pin().Add(context.Background(), path.New(cid+fpath))
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
	err = secret.Decrypt(SKeys.Identities, f, o)
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
	if SKeys == nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "getsecretkey first"))
		return
	}

	/// --- 1. 定义一个files.Nodes map,用于上传到IPFS网络
	postMap := map[string]files.Node{}
	//从请求中提取出请求内容
	var postform postForm
	err := c.ShouldBind(&postform)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("bind err: %s", err.Error())))
		return
	}

	/// --- 2. 解析出self发布的最新的cid，并写入到post中的next字段
	ipfskey, err := getIpfsKey(postform.KeyName)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("get  key err: %s", err.Error())))
		return
	}

	if !postform.Init {
		next, err := IpfsAPI.Name().Resolve(context.Background(), ipfskey.Path().String())
		if err != nil {
			c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("resolve err: %s", err.Error())))
			return
		}
		postform.Next = next.String()
	}

	/// --- 3. 获得组织加密公钥
	//如果postform.To 有值，那么加上自己的 secretkey ,否则自己是看不到自己发的消息的
	tos := []age.Recipient{}
	if len(postform.To) > 0 {
		postform.To = append(postform.To, SKeys.Recipient.(*age.X25519Recipient).String())
		for _, to := range postform.To {
			t, err := age.ParseX25519Recipient(to)
			if err != nil {
				continue
			}
			tos = append(tos, t)
		}
	}

	/// --- 4. 从请求内容中，提取出需要上传的文件，并写入到 postMap, 修改post中附件文件路径为文件名
	err = uploadFiles(&postform, postMap, tos)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	/// --- 5. 对post对象加密（如果不需要加密则保存原始数据）
	data, _ := json.Marshal(postform.post)
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
	metaJson, _ := json.Marshal(postform.meta)
	postMap[metaFile] = files.NewBytesFile(metaJson)

	/// --- 7. 将整个 postMap（包含post.json和所有附件）， 添加到IPFS网络,获得cid
	cid, err := IpfsAPI.Unixfs().Add(context.Background(), files.NewMapDirectory(postMap))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("add err: %s", err.Error())))
		return
	}

	/// --- 8. 发布新的cid到self IPNS
	nameEntry, err := IpfsAPI.Name().Publish(context.Background(), cid, options.Name.Key(ipfskey.Name()))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("publish err: %s", err.Error())))
		return
	}

	log.Println("name:", nameEntry.Name(), "value:", nameEntry.Value().String())
	//返回结果
	c.JSON(http.StatusOK, ResponseJsonFormat(1, map[string]string{
		"name":  nameEntry.Name(),
		"value": nameEntry.Value().String(),
	}))
}

// 从请求内容中，提取出需要上传的文件，并写入到 postMap, 修改post中附件文件路径为文件名
func uploadFiles(postform *postForm, postMap map[string]files.Node, tos []age.Recipient) error {
	//迭代所有附件
	//文件名不能为post,也不能重复
	// 如果有to，说明需要加密
	// 使用to里面的公钥加密文件
	// 获得文件加密后的数据（或者不需要加密的原始数据），并且保存到postMap
	for i, u := range postform.Uploads {
		log.Println(i, u.Filename)
		if _, ok := postMap[u.Filename]; u.Filename == indexFile || ok {
			return fmt.Errorf("filename err: %s %t %t", u.Filename, u.Filename == indexFile, ok)
		}

		postform.Attachments = append(postform.Attachments, u.Filename)

		f, err := u.Open()
		if err != nil {
			return fmt.Errorf("open file err: %s", err.Error())
		}

		var data []byte

		if len(postform.To) > 0 {

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
func newIpfsKeyHandler(c *gin.Context) {
	// options.NamePublishOption
	name := c.DefaultPostForm("name", "")
	key, err := IpfsAPI.Key().Generate(context.Background(), name)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, key.Path().String()))
}

// 移除ipfs key
func removeIpfsKeyHandler(c *gin.Context) {
	name := c.DefaultPostForm("name", "")
	key, err := IpfsAPI.Key().Remove(context.Background(), name)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, key.Path().String()))
}

// 列出所有IPFS key，也就是列出所有IPNS
func listIpfsKeyHandler(c *gin.Context) {

	keys, err := IpfsAPI.Key().List(context.Background())
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
	SKeys, err = secret.NewSecretKey(c.DefaultPostForm("oldpassword", ""), c.DefaultPostForm("password", ""))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, SKeys.Recipient.(*age.X25519Recipient).String()))
}

func getSecretKeyHandler(c *gin.Context) {
	var err error
	_, err = secret.GetSecretKey(c.DefaultPostForm("password", ""))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, SKeys.Recipient.(*age.X25519Recipient).String()))
}

// 获得用于加密的公钥
func getSecretRecipientHandler(c *gin.Context) {
	if SKeys == nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "getsecretkey first"))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, SKeys.Recipient.(*age.X25519Recipient).String()))
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

// 订阅其他人的IPNS name
func followHandler(c *gin.Context) {
	name := c.DefaultPostForm("name", "")
	addr := c.DefaultPostForm("addr", "")
	if addr == "" && name == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "null addr or name"))
		return
	}

	err := localstore.AddFollow(name, addr)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, "ok"))
}

// 添加其他人的Recipient
func addRecipientHandler(c *gin.Context) {
	name := c.DefaultPostForm("name", "")
	recipient := c.DefaultPostForm("recipient", "")
	peerPubKey := c.DefaultPostForm("peerpubkey", "")
	if name == "" && recipient == "" {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "null name or recipient"))
		return
	}

	err := localstore.AddPeer(name, recipient, peerPubKey)
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
					path, err := IpfsAPI.Name().Resolve(c, a.Addr)
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

func listenP2PHandler(c *gin.Context) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var readchan = make(chan []byte, 10)

	go func(readchan chan []byte) {
		defer close(readchan)
		if err := ipfsnode.ListenLocal(ctx, readchan, 8090, ipfsnode.MessageProto); err != nil {
			log.Println(err.Error())
		}

	}(readchan)

	c.Stream(func(w io.Writer) bool {
		if msg, ok := <-readchan; ok {
			err := localstore.WriteMessage(string(msg))
			if err != nil {
				c.SSEvent("message", err.Error())
				return false
			}
			c.SSEvent("message", msg)
			return true
		}
		return false
	})

}

func sendP2PHandler(c *gin.Context) {
	// 封装发送socket

	err := ipfsnode.SendMessage(
		c.DefaultPostForm("peerid", ""),
		c.DefaultPostForm("body", ""),
		8091,
		ipfsnode.MessageProto,
	)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(0, "ok"))
}

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

// 获得IPFS的key对象，如果传递的名称没有对应的key，默认返回self
func getIpfsKey(name string) (icore.Key, error) {
	ipnsKeys, err := IpfsAPI.Key().List(context.Background())
	if err != nil {
		return nil, err
	}

	for _, k := range ipnsKeys {
		if k.Name() == name {
			return k, nil
		}
	}

	return IpfsAPI.Key().Self(context.Background())
}

// json返回值对象
type responseJson struct {
	Code int8        `json:"code"`
	Data interface{} `json:"data"`
	Type string      `json:"type"`
}

// publish的表单request表单对象
type postForm struct {
	post                            //save to post.json
	meta                            //save to meta.json
	Uploads []*multipart.FileHeader `json:"-" form:"uploads"` //upload field
	KeyName string                  `json:"-" form:"keyname"` //which key to publish
	Init    bool                    `json:"-" form:"init"`
}

// 主内容对象
type post struct {
	Body        string   `json:"body" form:"body"`
	Type        string   `json:"type" form:"type"`
	Attachments []string `json:"attachments" form:"-"`
}

// 元数据对象，这个对象中的内容将不加密
type meta struct {
	To   []string `json:"to" form:"to"`
	Next string   `json:"next" form:"next"`
}

type listParams struct {
	Limit int `form:"limit"`
	Skip  int `form:"skip"`
}
